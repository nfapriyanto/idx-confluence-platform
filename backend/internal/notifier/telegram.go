package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/idx-trader/confluence-platform/internal/domain"
)

// =============================================================================
// MONOREPO PATH: backend/internal/notifier/telegram.go
// Notifikasi Real-Time Telegram Bot (Sinyal Grade A+ & Bandarmology Anomaly):
// 1. Mengirimkan Alert HTML terstruktur ke Telegram pribadi/channel saat emiten
//    menembus Grade A+ atau A pada mode Scalping, Swing, maupun Dividend Hunter.
// 2. Dilengkapi Cooldown De-duplicator per saham agar tidak mengirim spam
//    berulang-ulang pada sesi perdagangan yang sama.
// 3. Menyertakan rincian Skor 5 Lapis, Harga Modal Bandar, Area Entry, Stop Loss
//    berbasis ATR, Target TP1/TP2, dan rekomendasi porsi lot.
// =============================================================================

type TelegramNotifier struct {
	botToken   string
	chatID     string
	httpClient *http.Client
	mu         sync.Mutex
	lastSent   map[string]time.Time
	cooldown   time.Duration
}

func NewTelegramNotifierFromEnv() *TelegramNotifier {
	return &TelegramNotifier{
		botToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		chatID:     os.Getenv("TELEGRAM_CHAT_ID"),
		httpClient: &http.Client{Timeout: 8 * time.Second},
		lastSent:   make(map[string]time.Time),
		cooldown:   4 * time.Hour, // Maksimal 1 notifikasi per saham per sesi bursa
	}
}

func (t *TelegramNotifier) IsEnabled() bool {
	return t.botToken != "" && t.chatID != ""
}

type sendMessagePayload struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

// NotifyHighConvictionSignals memeriksa daftar sinyal dan mengirim alert untuk Grade A+
func (t *TelegramNotifier) NotifyHighConvictionSignals(ctx context.Context, signals []domain.ConfluenceResult) error {
	if !t.IsEnabled() {
		return nil
	}

	now := time.Now()
	for _, sig := range signals {
		if sig.Plan.SignalGrade != "A+" && sig.Plan.SignalGrade != "A" {
			continue
		}

		key := fmt.Sprintf("%s_%s", sig.Fundamental.Ticker, sig.Plan.RecommendedMode)
		t.mu.Lock()
		lastTime, exists := t.lastSent[key]
		if exists && now.Sub(lastTime) < t.cooldown {
			t.mu.Unlock()
			continue
		}
		t.lastSent[key] = now
		t.mu.Unlock()

		msg := FormatSignalAlertHTML(sig)
		if err := t.sendHTMLMessage(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// FormatSignalAlertHTML menyusun pesan Telegram lengkap dengan rincian 5 Lapis & Trade Plan
func FormatSignalAlertHTML(sig domain.ConfluenceResult) string {
	syariahBadge := ""
	if sig.Fundamental.IsSyariah {
		syariahBadge = " [ISSI Syariah]"
	}

	divNote := ""
	if sig.Dividend.HasUpcomingDividend {
		divNote = fmt.Sprintf(
			"\n💰 <b>Radar Dividen:</b> Yield %.2f%% (H-%d Cum Date)\n🛡️ <b>Trap Risk:</b> %.0f/100 → <i>%s</i>",
			sig.Dividend.EstimatedYieldPct,
			sig.Dividend.DaysToCumDate,
			sig.Plan.DividendTrapRisk,
			sig.Plan.DividendTactic,
		)
	}

	return fmt.Sprintf(
		"🚨 <b>IDX 5-LAYER CONFLUENCE ALERT [%s]</b>\n"+
			"📌 <b>%s</b> — %s%s\n"+
			"🎯 <b>Mode Terbaik:</b> %s | <b>Harga:</b> Rp %.0f\n\n"+
			"📊 <b>Bedah Skor 5 Lapis (0–100):</b>\n"+
			"• L1 Makro & GARCH Vol: <b>%.0f</b>\n"+
			"• L2 Valuasi Sektoral: <b>%.0f</b> (Fair: Rp %.0f)\n"+
			"• L3 Bandar & Foreign: <b>%.0f</b> (%s | Top Buyer: %s @ Rp %.0f)\n"+
			"• L4 Sentimen IndoBERT+Lex: <b>%.1f</b>\n"+
			"• L5 Technical Ensemble: <b>%.0f</b> (RVOL %.1fx | CRSI %.1f)%s\n\n"+
			"📐 <b>Rencana Eksekusi (Fraksi BEI & ATR-14):</b>\n"+
			"• <b>Area Beli:</b> Rp %.0f – Rp %.0f\n"+
			"• <b>Take Profit 1 & 2:</b> Rp %.0f / Rp %.0f\n"+
			"• <b>Stop Loss (ATR):</b> Rp %.0f (R/R 1:%.2f)\n"+
			"• <b>Alokasi Maksimal:</b> %.0f%% Portofolio",
		sig.Plan.SignalGrade,
		sig.Fundamental.Ticker,
		sig.Fundamental.CompanyName,
		syariahBadge,
		sig.Plan.RecommendedMode,
		sig.Fundamental.LastPrice,
		sig.Layer1MacroScore,
		sig.Layer2FundamentalScore,
		sig.IntrinsicFairVal,
		sig.Layer3SmartMoneyScore,
		sig.SmartMoney.BandarStatus,
		sig.SmartMoney.Top1BuyerCode,
		sig.SmartMoney.BandarAvgBuyPrice,
		sig.Layer4SentimentScore,
		sig.Layer5TechnicalScore,
		sig.Technical.IntradayRVOL,
		sig.Technical.ConnorsRSI3,
		divNote,
		sig.Plan.EntryZoneMin,
		sig.Plan.EntryZoneMax,
		sig.Plan.TakeProfit1,
		sig.Plan.TakeProfit2,
		sig.Plan.StopLossPrice,
		sig.Plan.RiskRewardRatio,
		sig.Plan.MaxPortfolioAllocPct,
	)
}

func (t *TelegramNotifier) sendHTMLMessage(ctx context.Context, htmlText string) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
	bodyBytes, err := json.Marshal(sendMessagePayload{
		ChatID:                t.chatID,
		Text:                  htmlText,
		ParseMode:             "HTML",
		DisableWebPagePreview: true,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram API status %d", resp.StatusCode)
	}
	return nil
}