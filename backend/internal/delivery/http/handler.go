package httpdelivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/idx-trader/confluence-platform/internal/backtest"
	"github.com/idx-trader/confluence-platform/internal/domain"
	"github.com/idx-trader/confluence-platform/internal/nlp"
	"github.com/idx-trader/confluence-platform/internal/notifier"
	"github.com/idx-trader/confluence-platform/internal/providers/idx"
	"github.com/idx-trader/confluence-platform/internal/providers/ipot"
	"github.com/idx-trader/confluence-platform/internal/providers/rss"
	"github.com/idx-trader/confluence-platform/internal/providers/yahoo"
	"github.com/idx-trader/confluence-platform/internal/strategy"
)

// =============================================================================
// MONOREPO PATH: backend/internal/delivery/http/handler.go
// Handler REST API (/api/v1/signals, /api/v1/backtest), Live SSE (/api/v1/stream),
// Telegram Alert Trigger, & Orkestrasi Jadwal Jam Bursa BEI
// =============================================================================

type Handler struct {
	mu          sync.RWMutex
	db          *sql.DB
	nlpEngine   *nlp.Engine
	yahooClient *yahoo.Client
	idxClient   *idx.Client
	ipotClient  *ipot.Client
	rssClient   *rss.Client
	tgNotifier  *notifier.TelegramNotifier
	wibLoc      *time.Location
	Macro       domain.MacroState         `json:"macro"`
	Signals     []domain.ConfluenceResult `json:"signals"`
}

func NewHandler(
	db *sql.DB,
	nlpEngine *nlp.Engine,
	yahooClient *yahoo.Client,
	idxClient *idx.Client,
	ipotClient *ipot.Client,
	rssClient *rss.Client,
	macro domain.MacroState,
	signals []domain.ConfluenceResult,
) *Handler {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	return &Handler{
		db:          db,
		nlpEngine:   nlpEngine,
		yahooClient: yahooClient,
		idxClient:   idxClient,
		ipotClient:  ipotClient,
		rssClient:   rssClient,
		tgNotifier:  notifier.NewTelegramNotifierFromEnv(),
		wibLoc:      loc,
		Macro:       macro,
		Signals:     signals,
	}
}

func (h *Handler) IsIDXTradingHours(now time.Time) bool {
	wib := now.In(h.wibLoc)
	wd := wib.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	mins := wib.Hour()*60 + wib.Minute()
	s1End := 720
	s2Start := 810
	if wd == time.Friday {
		s1End = 690
		s2Start = 840
	}
	return (mins >= 540 && mins <= s1End) || (mins >= s2Start && mins <= 950)
}

func (h *Handler) StartIDXMarketScheduler(ctx context.Context) {
	minuteTicker := time.NewTicker(1 * time.Minute)
	rssTicker := time.NewTicker(10 * time.Minute)

	go func() {
		defer minuteTicker.Stop()
		defer rssTicker.Stop()
		var lastPostMarketDate string

		for {
			select {
			case <-ctx.Done():
				return
			case now := <-minuteTicker.C:
				wib := now.In(h.wibLoc)
				if h.IsIDXTradingHours(wib) {
					h.RefreshConfluenceScores(ctx)
				}
				dateKey := wib.Format("20060102")
				if wib.Weekday() != time.Saturday && wib.Weekday() != time.Sunday &&
					wib.Hour() == 16 && wib.Minute() >= 35 && lastPostMarketDate != dateKey {
					log.Printf("[Post-Market 16:35 WIB] Menyinkronkan Foreign Flow IDX & Broker Summary IPOT (%s)...", dateKey)
					h.RefreshConfluenceScores(ctx)
					lastPostMarketDate = dateKey
				}
			case <-rssTicker.C:
				h.RefreshConfluenceScores(ctx)
			}
		}
	}()
}

func (h *Handler) RefreshConfluenceScores(ctx context.Context) {
	h.mu.Lock()
	updated := make([]domain.ConfluenceResult, len(h.Signals))
	for i, item := range h.Signals {
		updated[i] = strategy.EvaluateWithNLP(
			h.Macro,
			item.Fundamental,
			item.SmartMoney,
			item.RecentNews,
			item.Dividend,
			item.Technical,
			h.nlpEngine,
		)
	}
	h.Signals = updated
	h.mu.Unlock()

	if h.tgNotifier != nil && h.tgNotifier.IsEnabled() {
		_ = h.tgNotifier.NotifyHighConvictionSignals(ctx, updated)
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// 1. Endpoint Sinyal Konfluensi 5 Lapis
	mux.HandleFunc("/api/v1/signals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		h.mu.RLock()
		defer h.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(h)
	})

	// 2. Endpoint Simulator Backtesting Walk-Forward (Fee Beli 0.15%, Fee Jual 0.25%)
	mux.HandleFunc("/api/v1/backtest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		ticker := r.URL.Query().Get("ticker")
		if ticker == "" {
			ticker = "BMRI"
		}

		h.mu.RLock()
		var targetSig domain.ConfluenceResult
		if len(h.Signals) > 0 {
			targetSig = h.Signals[0]
		}
		for _, s := range h.Signals {
			if s.Fundamental.Ticker == ticker {
				targetSig = s
				break
			}
		}
		h.mu.RUnlock()

		history := generateSampleWalkForwardHistory(targetSig)
		report := backtest.RunWalkForwardBacktest(ticker, history, backtest.DefaultIDXConfig())
		_ = json.NewEncoder(w).Encode(report)
	})

	// 3. Endpoint Server-Sent Events (SSE) Live Intraday Stream
	mux.HandleFunc("/api/v1/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE unsupported", http.StatusInternalServerError)
			return
		}
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-t.C:
				h.mu.RLock()
				payload, _ := json.Marshal(h)
				h.mu.RUnlock()
				fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			}
		}
	})
}

func generateSampleWalkForwardHistory(base domain.ConfluenceResult) []backtest.HistoricalDayInput {
	var out []backtest.HistoricalDayInput
	price := base.Fundamental.LastPrice * 0.86
	startTime := time.Now().AddDate(0, -6, 0)

	for i := 0; i < 120; i++ {
		dt := startTime.AddDate(0, 0, i)
		wave := 1.0 + 0.012*float64((i%9)-4)
		openP := domain.RoundToIDXTick(price)
		closeP := domain.RoundToIDXTick(price * wave)
		highP := domain.RoundToIDXTick(closeP * 1.018)
		lowP := domain.RoundToIDXTick(openP * 0.988)
		price = closeP

		sig := base
		sig.Fundamental.LastPrice = closeP
		sig.Plan.StopLossPrice = domain.RoundToIDXTick(closeP * 0.96)
		sig.Plan.TakeProfit2 = domain.RoundToIDXTick(closeP * 1.065)

		out = append(out, backtest.HistoricalDayInput{
			Date: dt,
			Bar: domain.OHLCVBar{
				Timestamp: dt,
				Ticker:    base.Fundamental.Ticker,
				Open:      openP,
				High:      highP,
				Low:       lowP,
				Close:     closeP,
				Volume:    45_000_000,
			},
			Confluence: sig,
		})
	}
	return out
}