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

	"github.com/idx-trader/confluence-platform/internal/domain"
	"github.com/idx-trader/confluence-platform/internal/nlp"
	"github.com/idx-trader/confluence-platform/internal/providers/idx"
	"github.com/idx-trader/confluence-platform/internal/providers/ipot"
	"github.com/idx-trader/confluence-platform/internal/providers/rss"
	"github.com/idx-trader/confluence-platform/internal/providers/yahoo"
	"github.com/idx-trader/confluence-platform/internal/strategy"
)

// =============================================================================
// MONOREPO PATH: backend/internal/delivery/http/handler.go
// Handler REST API, Server-Sent Events (SSE), & Orkestrasi Jadwal Jam Bursa BEI
// =============================================================================

type Handler struct {
	mu          sync.RWMutex
	db          *sql.DB
	nlpEngine   *nlp.Engine
	yahooClient *yahoo.Client
	idxClient   *idx.Client
	ipotClient  *ipot.Client
	rssClient   *rss.Client
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
		wibLoc:      loc,
		Macro:       macro,
		Signals:     signals,
	}
}

// IsIDXTradingHours memeriksa apakah waktu saat ini berada di Sesi 1 atau Sesi 2 BEI
func (h *Handler) IsIDXTradingHours(now time.Time) bool {
	wib := now.In(h.wibLoc)
	wd := wib.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	mins := wib.Hour()*60 + wib.Minute()
	s1End := 720 // 12:00 WIB
	s2Start := 810 // 13:30 WIB
	if wd == time.Friday {
		s1End = 690 // 11:30 WIB
		s2Start = 840 // 14:00 WIB
	}
	return (mins >= 540 && mins <= s1End) || (mins >= s2Start && mins <= 950)
}

// StartIDXMarketScheduler menjalankan cron otomatis untuk Scalping (1m), RSS (10m), dan Bandarmology (16:35 WIB)
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
	defer h.mu.Unlock()

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
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/signals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		h.mu.RLock()
		defer h.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(h)
	})

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