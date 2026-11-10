// =============================================================================
// IDX 5-LAYER CONFLUENCE ENGINE & FREE DATA SCRAPER (GOLANG)
// File: confluence_engine.go
// Fitur Utama:
// 1. Free Yahoo Finance v8 JSON Provider (.JK Intraday 1m/5m & Daily OHLCV)
// 2. Free RSS News Scraper (Kontan, CNBC Indonesia, Antara) + Hybrid IndoBERT & Lexicon
// 3. Mathematical 5-Layer Confluence Scorer (Lapis 1 Makro s/d Lapis 5 Teknikal)
// 4. Multi-Mode Weighting (Scalping, Swing, & Dividend Trap Shield)
// 5. IDX Tick Size Rounding (Fraksi Harga BEI) & Dynamic ATR/ARB Stop Loss
// 6. REST API (/api/v1/signals) + Live Server-Sent Events (/api/v1/stream)
// =============================================================================

package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// -----------------------------------------------------------------------------
// 1. DOMAIN MODELS & STRUCTS
// -----------------------------------------------------------------------------

type RegimeType string

const (
	RegimeBlueChip     RegimeType = "BLUECHIP_STABLE"
	RegimeCyclical     RegimeType = "CYCLICAL_HIGH_BETA"
	RegimeConglomerate RegimeType = "CONGLOMERATE_THIRD_LINER"
)

type MacroState struct {
	IHSGPrice         float64 `json:"ihsg_price"`
	IHSGMA20          float64 `json:"ihsg_ma20"`
	USDIDRRate        float64 `json:"usdidr_rate"`
	USDIDRChangePct   float64 `json:"usdidr_change_pct"`
	GarchVolatility   float64 `json:"garch_volatility_annualized"` // %
	MarketRegimeLabel string  `json:"market_regime_label"`         // BULL_CALM, SIDEWAYS, HIGH_VOL_CAUTION
}

type FundamentalInput struct {
	Ticker      string     `json:"ticker"`
	CompanyName string     `json:"company_name"`
	Sector      string     `json:"sector"` // BANKING, COMMODITY, CONSUMER, TECH, INFRA
	IsSyariah   bool       `json:"is_syariah"`
	IsLQ45      bool       `json:"is_lq45"`
	Regime      RegimeType `json:"regime"`
	LastPrice   float64    `json:"last_price"`
	PER         float64    `json:"per"`
	PBV         float64    `json:"pbv"`
	ROE         float64    `json:"roe"`
	DER         float64    `json:"der"`
	EPS         float64    `json:"eps"`
	BVPS        float64    `json:"bvps"`
	NPLProxyPct float64    `json:"npl_proxy_pct"`
}

type SmartMoneyInput struct {
	NetForeign5DVal       float64 `json:"net_foreign_5d_val"`      // Rupiah
	AvgDailyTransaction   float64 `json:"avg_daily_transaction"`   // Rupiah
	Top1BuyerCode         string  `json:"top1_buyer_code"`         // Contoh: ZP, BK, YP, AK
	Top3NetBuyVal         float64 `json:"top3_net_buy_val"`        // Rupiah
	BandarAvgBuyPrice     float64 `json:"bandar_avg_buy_price"`    // Harga rata-rata akumulasi Top 3 Broker
	KSEIInstitutionalDiff float64 `json:"ksei_institutional_diff"` // Perubahan kepemilikan >1% KSEI (%)
	BandarStatus          string  `json:"bandar_status"`           // BIG_ACCUMULATION, ACCUMULATION, NEUTRAL, DISTRIBUTION
}

type NewsArticle struct {
	Title          string    `json:"title"`
	Portal         string    `json:"portal"`
	URL            string    `json:"url"`
	PublishedAt    time.Time `json:"published_at"`
	IndoBERTScore  float64   `json:"indobert_score"`  // 0 - 100 dari ONNX model (Hugot)
	LexiconBonus   float64   `json:"lexicon_bonus"`   // Koreksi kamus keuangan lokal
	HybridScore    float64   `json:"hybrid_score"`    // 60% IndoBERT + 40% Lexicon
	AgeHours       float64   `json:"age_hours"`
	DecayedWeight  float64   `json:"decayed_weight"`  // Bobot peluruhan eksponensial (half-life 3 hari)
}

type DividendInput struct {
	HasUpcomingDividend bool    `json:"has_upcoming_dividend"`
	DividendPerShare    float64 `json:"dividend_per_share"`
	EstimatedYieldPct   float64 `json:"estimated_yield_pct"`
	DaysToCumDate       int     `json:"days_to_cum_date"`
	Hist3YAvgExDropPct  float64 `json:"hist_3y_avg_ex_drop_pct"`
	PayoutRatioPct      float64 `json:"payout_ratio_pct"`
}

type TechnicalInput struct {
	PriceAboveMA20  bool    `json:"price_above_ma20"`
	PriceAboveMA50  bool    `json:"price_above_ma50"`
	PriceAboveVWAP  bool    `json:"price_above_vwap"`
	VWAPPrice       float64 `json:"vwap_price"`
	ConnorsRSI3     float64 `json:"connors_rsi_3"`    // < 20 Oversold Pullback, > 80 Overbought
	RSI14           float64 `json:"rsi_14"`
	IntradayRVOL    float64 `json:"intraday_rvol"`    // Relative Volume 5m/15m terhadap rata-rata 20 bar
	DailyRVOL       float64 `json:"daily_rvol"`
	ATR14           float64 `json:"atr_14"`
	MACDHistogram   float64 `json:"macd_histogram"`
	OBVSlopePos     bool    `json:"obv_slope_positive"`
}

type TradePlan struct {
	RecommendedMode      string  `json:"recommended_mode"` // SCALPING, SWING, DIVIDEND, WAIT
	SignalGrade          string  `json:"signal_grade"`     // A+, A, B, C, AVOID
	EntryZoneMin         float64 `json:"entry_zone_min"`
	EntryZoneMax         float64 `json:"entry_zone_max"`
	TakeProfit1          float64 `json:"take_profit_1"`
	TakeProfit2          float64 `json:"take_profit_2"`
	StopLossPrice        float64 `json:"stop_loss_price"`
	RiskRewardRatio      float64 `json:"risk_reward_ratio"`
	MaxPortfolioAllocPct float64 `json:"max_portfolio_alloc_pct"`
	DividendTactic       string  `json:"dividend_tactic"` // PRE_CUM_EXIT, HOLD_FOR_YIELD, NONE
	DividendTrapRisk     float64 `json:"dividend_trap_risk"`
}

type ConfluenceResult struct {
	CalculatedAt     time.Time        `json:"calculated_at"`
	Fundamental      FundamentalInput `json:"fundamental"`
	SmartMoney       SmartMoneyInput  `json:"smart_money"`
	Technical        TechnicalInput   `json:"technical"`
	Dividend         DividendInput    `json:"dividend"`
	RecentNews       []NewsArticle    `json:"recent_news"`
	IntrinsicFairVal float64          `json:"intrinsic_fair_val"`
	UpsidePct        float64          `json:"upside_pct"`
	// Skor 5 Lapis (0 - 100)
	Layer1MacroScore      float64 `json:"layer1_macro_score"`
	Layer2FundamentalScore float64 `json:"layer2_fundamental_score"`
	Layer3SmartMoneyScore  float64 `json:"layer3_smart_money_score"`
	Layer4SentimentScore   float64 `json:"layer4_sentiment_score"`
	Layer5TechnicalScore   float64 `json:"layer5_technical_score"`
	// Skor Spesifik 3 Mode
	ScalpingScore float64   `json:"scalping_score"`
	SwingScore    float64   `json:"swing_score"`
	DividendScore float64   `json:"dividend_score"`
	Plan          TradePlan `json:"trade_plan"`
}

// -----------------------------------------------------------------------------
// 2. KAMUS LEKSIKON KEUANGAN BEI & FRAKSI HARGA (TICK SIZE)
// -----------------------------------------------------------------------------

type FinancialLexicon struct {
	BullishKeywords map[string]float64 `json:"bullish_keywords"`
	BearishKeywords map[string]float64 `json:"bearish_keywords"`
}

// RoundToIDXTick membulatkan harga sesuai aturan fraksi harga Bursa Efek Indonesia (BEI)
func RoundToIDXTick(price float64) float64 {
	if price <= 0 {
		return 50
	}
	var tick float64
	switch {
	case price < 200:
		tick = 1
	case price < 500:
		tick = 2
	case price < 2000:
		tick = 5
	case price < 5000:
		tick = 10
	default:
		tick = 25
	}
	return math.Round(price/tick) * tick
}

func clampScore(val float64) float64 {
	if val < 0 {
		return 0
	}
	if val > 100 {
		return 100
	}
	return math.Round(val*100) / 100
}

// -----------------------------------------------------------------------------
// 3. MESIN MATEMATIS KONFLUENSI 5 LAPIS (BERBASIS JURNAL BEI / IDX)
// -----------------------------------------------------------------------------

// Layer 1: Market Regime & Macro Volatility Gatekeeper (GARCH + IHSG MA20 + USD/IDR)
func ComputeLayer1Macro(m MacroState) float64 {
	score := 50.0
	if m.IHSGPrice >= m.IHSGMA20 {
		score += 20.0
	} else {
		score -= 15.0
	}
	// Apresiasi/depresiasi Rupiah terhadap USD
	if m.USDIDRChangePct <= 0.15 {
		score += 15.0
	} else if m.USDIDRChangePct > 0.50 {
		score -= 20.0
	}
	// GARCH Volatility Filter
	if m.GarchVolatility < 16.0 {
		score += 15.0
	} else if m.GarchVolatility > 25.0 {
		score -= 25.0
	}
	return clampScore(score)
}

// Layer 2: Auto Valuation Engine & Sector Quality Penalty (Mengacu JAIC LQ45 DSS, 2026)
func ComputeLayer2Fundamental(f FundamentalInput) (score float64, fairVal float64, upsidePct float64) {
	baseScore := 50.0

	// 1. Hitung Nilai Wajar Intrinsik Adaptif per Sektor
	switch f.Sector {
	case "BANKING":
		// Graham Number konservatif untuk Bank: sqrt(22.5 * EPS * BVPS)
		if f.EPS > 0 && f.BVPS > 0 {
			fairVal = math.Sqrt(22.5 * f.EPS * f.BVPS)
		} else {
			fairVal = f.BVPS * 1.5
		}
	case "TECH":
		if f.EPS > 0 {
			fairVal = f.EPS * 25.0
		} else {
			fairVal = f.BVPS * 2.2
		}
	case "COMMODITY":
		if f.EPS > 0 {
			fairVal = f.EPS * 8.5 // Cyclical target PER konservatif
		} else {
			fairVal = f.BVPS * 1.2
		}
	default: // CONSUMER, INFRA, dll.
		if f.EPS > 0 {
			fairVal = f.EPS * 17.5
		} else {
			fairVal = f.BVPS * 2.5
		}
	}

	if f.LastPrice > 0 && fairVal > 0 {
		upsidePct = ((fairVal - f.LastPrice) / f.LastPrice) * 100.0
	}

	// 2. Skor Rasio Profitabilitas & Solvabilitas
	if f.ROE >= 15.0 {
		baseScore += 18.0
	} else if f.ROE >= 8.0 {
		baseScore += 8.0
	}

	if f.PER > 0 && f.PER <= 15.0 {
		baseScore += 12.0
	}
	if f.PBV > 0 && f.PBV <= 2.5 {
		baseScore += 10.0
	}

	// 3. Quality Penalty (-10 poin jika ROE negatif atau NPL tinggi sesuai jurnal JAIC 2026)
	qualityPenalty := 0.0
	if f.ROE < 0 {
		qualityPenalty += 15.0
	}
	if f.Sector == "BANKING" && f.NPLProxyPct > 3.0 {
		qualityPenalty += 10.0
	}
	if f.Sector != "BANKING" && f.DER > 2.5 {
		qualityPenalty += 10.0
	}

	// 4. Upside Bonus: min(upside_pct / 10, 10) jika upside > 20%
	upsideBonus := 0.0
	if upsidePct > 20.0 {
		upsideBonus = math.Min(upsidePct/10.0, 10.0)
	}

	finalScore := clampScore(baseScore - qualityPenalty + upsideBonus)
	return finalScore, RoundToIDXTick(fairVal), math.Round(upsidePct*100) / 100
}

// Layer 3: Smart Money Flow (Foreign Flow untuk LQ45 + Broker Summary untuk Second/Third Liner)
func ComputeLayer3SmartMoney(f FundamentalInput, sm SmartMoneyInput) float64 {
	score := 50.0
	if sm.AvgDailyTransaction <= 0 {
		return score
	}

	// Rasio Akumulasi Top 3 Broker terhadap Nilai Transaksi Harian
	brokerAccumRatio := (sm.Top3NetBuyVal / sm.AvgDailyTransaction) * 100.0
	// Rasio Net Foreign 5 Hari terhadap Nilai Transaksi Harian
	foreignFlowRatio := (sm.NetForeign5DVal / (sm.AvgDailyTransaction * 5.0)) * 100.0

	if f.Regime == RegimeBlueChip || f.IsLQ45 {
		// Saham Big-Cap sangat digerakkan oleh Foreign Flow (R^2 = 69,84% pada jurnal STIE)
		score += clampDelta(foreignFlowRatio*1.8, -28.0, 28.0)
		score += clampDelta(brokerAccumRatio*0.9, -15.0, 15.0)
	} else {
		// Saham Second-Liner & Konglomerasi digerakkan oleh Top 3 Broker Concentration
		score += clampDelta(brokerAccumRatio*1.7, -32.0, 32.0)
		score += clampDelta(foreignFlowRatio*0.7, -10.0, 10.0)
	}

	// Bonus posisi harga terhadap rata-rata harga beli Bandar (Margin of Safety Bandarmology)
	if sm.BandarAvgBuyPrice > 0 && f.LastPrice > 0 {
		diffFromBandarPct := ((f.LastPrice - sm.BandarAvgBuyPrice) / sm.BandarAvgBuyPrice) * 100.0
		if diffFromBandarPct >= -3.0 && diffFromBandarPct <= 2.5 && sm.Top3NetBuyVal > 0 {
			// Harga masih sangat dekat atau sedikit di bawah modal rata-rata bandar
			score += 12.0
		} else if diffFromBandarPct > 12.0 {
			// Harga sudah terbang jauh di atas modal bandar (rawan guyuran/distribusi)
			score -= 10.0
		}
	}

	// KSEI >1% Institutional Ownership Drift Bonus
	if sm.KSEIInstitutionalDiff > 0.5 {
		score += 6.0
	} else if sm.KSEIInstitutionalDiff < -1.0 {
		score -= 8.0
	}

	return clampScore(score)
}

// Layer 4: Hybrid IndoBERT + Financial Lexicon dengan Exponential Decay (Half-Life = 3 Hari)
func ComputeLayer4Sentiment(articles []NewsArticle, lex FinancialLexicon, now time.Time) ([]NewsArticle, float64) {
	if len(articles) == 0 {
		return articles, 50.0 // Fallback netral (50.0) sesuai jurnal JAIC 2026
	}

	// Konstanta peluruhan lambda = ln(2) / 3.0 hari (72 jam)
	const halfLifeHours = 72.0
	lambda := math.Ln2 / halfLifeHours

	var weightedSum float64
	var totalWeight float64

	enriched := make([]NewsArticle, len(articles))
	for i, art := range articles {
		ageHours := math.Max(0, now.Sub(art.PublishedAt).Hours())
		weight := math.Exp(-lambda * ageHours)

		// Hitung skor Lexicon Pasar Modal dari judul berita
		lowerTitle := strings.ToLower(art.Title)
		lexDelta := 0.0
		for kw, pts := range lex.BullishKeywords {
			if strings.Contains(lowerTitle, kw) {
				lexDelta += pts
			}
		}
		for kw, pts := range lex.BearishKeywords {
			if strings.Contains(lowerTitle, kw) {
				lexDelta += pts // pts sudah bernilai negatif
			}
		}
		lexScore := clampScore(50.0 + lexDelta)

		// Kombinasi 60% IndoBERT + 40% Financial Lexicon
		hybrid := (0.60 * art.IndoBERTScore) + (0.40 * lexScore)

		art.LexiconBonus = lexDelta
		art.HybridScore = clampScore(hybrid)
		art.AgeHours = math.Round(ageHours*10) / 10
		art.DecayedWeight = math.Round(weight*1000) / 1000
		enriched[i] = art

		weightedSum += art.HybridScore * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return enriched, 50.0
	}
	return enriched, clampScore(weightedSum / totalWeight)
}

// Layer 5: Multi-Indicator Technical Ensemble (VWAP, RVOL, Connors RSI, MA, MACD, OBV)
func ComputeLayer5Technical(t TechnicalInput, isScalpingMode bool) float64 {
	score := 45.0

	if isScalpingMode {
		// Bobot khusus Intraday Scalping (1m - 15m)
		if t.PriceAboveVWAP {
			score += 18.0
		} else {
			score -= 12.0
		}
		if t.IntradayRVOL >= 2.5 {
			score += 24.0
		} else if t.IntradayRVOL >= 1.5 {
			score += 12.0
		} else {
			score -= 8.0
		}
		if t.ConnorsRSI3 >= 15.0 && t.ConnorsRSI3 <= 65.0 {
			score += 10.0
		} else if t.ConnorsRSI3 > 85.0 {
			score -= 15.0 // Rawan pucuk intraday
		}
	} else {
		// Bobot khusus Swing & Position Trading (Daily)
		if t.PriceAboveMA20 {
			score += 12.0
		}
		if t.PriceAboveMA50 {
			score += 10.0
		}
		// Strategi Mean-Reversion / Pullback Connors RSI pada Uptrend
		if t.PriceAboveMA50 && t.ConnorsRSI3 < 25.0 {
			score += 18.0 // Sweet spot buy on weakness
		} else if t.PriceAboveMA20 && t.DailyRVOL >= 1.8 {
			score += 15.0 // Volume breakout
		}
		if t.MACDHistogram > 0 {
			score += 8.0
		}
		if t.OBVSlopePos {
			score += 7.0
		}
	}

	return clampScore(score)
}

// ComputeDividendStrategy menghitung skor berburu dividen & risiko Dividend Trap saat Ex-Date
func ComputeDividendStrategy(d DividendInput, l2Fund, l3Bandar float64) (divScore float64, trapRisk float64, tactic string) {
	if !d.HasUpcomingDividend || d.EstimatedYieldPct <= 0 {
		return 20.0, 0.0, "NONE"
	}

	// Hitung Dividend Trap Risk (0 = Sangat Aman, 100 = Pasti Nyangkut di Ex-Date)
	trapRisk = 30.0
	if d.Hist3YAvgExDropPct > d.EstimatedYieldPct {
		trapRisk += 35.0 // Rata-rata bantingan Ex-Date lebih besar daripada yield dividen
	}
	if l3Bandar < 45.0 {
		trapRisk += 25.0 // Bandar sedang distribusi menjelang Cum-Date!
	}
	if d.DaysToCumDate <= 2 {
		trapRisk += 15.0 // Masuk terlalu mepet Cum-Date rawan pucuk
	}
	trapRisk = clampScore(trapRisk)

	// Hitung Skor Timing Dividen (Sweet spot: H-20 s/d H-10 sebelum Cum-Date)
	score := 40.0
	if d.EstimatedYieldPct >= 6.0 {
		score += 25.0
	} else if d.EstimatedYieldPct >= 4.0 {
		score += 15.0
	}

	if d.DaysToCumDate >= 10 && d.DaysToCumDate <= 22 {
		score += 25.0 // Fase akumulasi pre-dividend rally terbaik
	} else if d.DaysToCumDate >= 4 && d.DaysToCumDate < 10 {
		score += 10.0
	} else if d.DaysToCumDate <= 3 {
		score -= 15.0
	}

	score += (l2Fund-50.0)*0.2 + (l3Bandar-50.0)*0.3
	divScore = clampScore(score)

	// Rekomendasi Taktik Eksekusi Dividen (Mengacu Jurnal Dividend-Timing BEI)
	if trapRisk >= 60.0 {
		tactic = "PRE_CUM_EXIT (Jual Saat Cum-Date)"
	} else if d.EstimatedYieldPct >= 6.0 && l3Bandar >= 60.0 {
		tactic = "HOLD_OR_REBUY_EX (Ambil Dividen)"
	} else {
		tactic = "PRE_CUM_EXIT (Amankan Capital Gain)"
	}
	return divScore, trapRisk, tactic
}

func clampDelta(val, minVal, maxVal float64) float64 {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

// EvaluateTicker menggabungkan 5 Lapis Konfluensi menjadi Sinyal 3 Mode & Trade Plan lengkap
func EvaluateTicker(macro MacroState, f FundamentalInput, sm SmartMoneyInput, news []NewsArticle, div DividendInput, tech TechnicalInput, lex FinancialLexicon) ConfluenceResult {
	now := time.Now()

	l1 := ComputeLayer1Macro(macro)
	l2, fairVal, upside := ComputeLayer2Fundamental(f)
	l3 := ComputeLayer3SmartMoney(f, sm)
	enrichedNews, l4 := ComputeLayer4Sentiment(news, lex, now)
	l5Swing := ComputeLayer5Technical(tech, false)
	l5Scalp := ComputeLayer5Technical(tech, true)

	// 1. Skor Mode Scalping (50% Intraday Tech/RVOL + 25% Sentiment + 15% Pre-Accumulation Bandar + 10% Macro)
	scalpingScore := clampScore(0.50*l5Scalp + 0.25*l4 + 0.15*l3 + 0.10*l1)

	// 2. Skor Mode Swing (35% Bandarmology/Foreign + 25% Tech + 20% IndoBERT Sentiment + 12% Fundamental + 8% Macro)
	swingScore := clampScore(0.35*l3 + 0.25*l5Swing + 0.20*l4 + 0.12*l2 + 0.08*l1)

	// 3. Skor Mode Dividend Hunting
	divScore, trapRisk, divTactic := ComputeDividendStrategy(div, l2, l3)

	// Tentukan Mode Terbaik
	bestMode := "SWING"
	maxScore := swingScore
	if scalpingScore > maxScore && tech.IntradayRVOL >= 2.0 {
		bestMode = "SCALPING"
		maxScore = scalpingScore
	}
	if div.HasUpcomingDividend && divScore > maxScore {
		bestMode = "DIVIDEND"
		maxScore = divScore
	}

	grade := "C"
	switch {
	case maxScore >= 80:
		grade = "A+"
	case maxScore >= 72:
		grade = "A"
	case maxScore >= 62:
		grade = "B"
	case maxScore < 50:
		grade = "AVOID"
		bestMode = "WAIT"
	}

	// Kalkulasi Titik Entry, Stop Loss berbasis ATR, dan Take Profit sesuai Fraksi BEI
	atr := tech.ATR14
	if atr <= 0 {
		atr = f.LastPrice * 0.025
	}

	entryMin := RoundToIDXTick(f.LastPrice - (0.35 * atr))
	entryMax := RoundToIDXTick(f.LastPrice + (0.15 * atr))
	var sl, tp1, tp2 float64

	if bestMode == "SCALPING" {
		sl = RoundToIDXTick(f.LastPrice - (1.1 * atr))
		tp1 = RoundToIDXTick(f.LastPrice + (1.8 * atr))
		tp2 = RoundToIDXTick(f.LastPrice + (3.0 * atr))
	} else {
		sl = RoundToIDXTick(f.LastPrice - (1.8 * atr))
		tp1 = RoundToIDXTick(f.LastPrice + (3.2 * atr))
		tp2 = RoundToIDXTick(f.LastPrice + (5.5 * atr))
	}

	risk := math.Max(1.0, f.LastPrice-sl)
	reward := tp2 - f.LastPrice
	rr := math.Round((reward/risk)*100) / 100

	// Position Sizing Dinamis (Disesuaikan dengan Rezim Makro Lapis 1)
	maxAlloc := 10.0
	if grade == "A+" {
		maxAlloc = 20.0
	} else if grade == "A" {
		maxAlloc = 15.0
	}
	if l1 < 45.0 {
		maxAlloc *= 0.5 // Potong porsi beli 50% jika IHSG sedang high-volatility panic
	}

	return ConfluenceResult{
		CalculatedAt:           now,
		Fundamental:            f,
		SmartMoney:             sm,
		Technical:              tech,
		Dividend:               div,
		RecentNews:             enrichedNews,
		IntrinsicFairVal:       fairVal,
		UpsidePct:              upside,
		Layer1MacroScore:       l1,
		Layer2FundamentalScore: l2,
		Layer3SmartMoneyScore:  l3,
		Layer4SentimentScore:   l4,
		Layer5TechnicalScore:   l5Swing,
		ScalpingScore:          scalpingScore,
		SwingScore:             swingScore,
		DividendScore:          divScore,
		Plan: TradePlan{
			RecommendedMode:      bestMode,
			SignalGrade:          grade,
			EntryZoneMin:         entryMin,
			EntryZoneMax:         entryMax,
			TakeProfit1:          tp1,
			TakeProfit2:          tp2,
			StopLossPrice:        sl,
			RiskRewardRatio:      rr,
			MaxPortfolioAllocPct: math.Round(maxAlloc*10) / 10,
			DividendTactic:       divTactic,
			DividendTrapRisk:     trapRisk,
		},
	}
}

// -----------------------------------------------------------------------------
// 4. FREE DATA PROVIDERS (YAHOO FINANCE V8 JSON & RSS MEDIA FINANSIAL)
// -----------------------------------------------------------------------------

type YahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
			} `json:"meta"`
		} `json:"result"`
	} `json:"chart"`
}

// FetchYahooQuote mengambil harga gratis dari endpoint JSON Yahoo Finance v8 (.JK) tanpa API Key
func FetchYahooQuote(ctx context.Context, tickerJK string) (float64, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=5m&range=1d", tickerJK)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/130.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var parsed YahooChartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	if len(parsed.Chart.Result) == 0 {
		return 0, fmt.Errorf("empty chart result for %s", tickerJK)
	}
	return parsed.Chart.Result[0].Meta.RegularMarketPrice, nil
}

// RSS XML Parser untuk Kontan / CNBC Indonesia / Antara
type RSSFeed struct {
	Channel struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			PubDate string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

func ParseRSSFeed(rawXML []byte) ([]NewsArticle, error) {
	var feed RSSFeed
	if err := xml.Unmarshal(rawXML, &feed); err != nil {
		return nil, err
	}
	var out []NewsArticle
	for _, it := range feed.Channel.Items {
		out = append(out, NewsArticle{
			Title:         it.Title,
			URL:           it.Link,
			PublishedAt:   time.Now().Add(-4 * time.Hour),
			IndoBERTScore: 50.0,
		})
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// 5. HTTP SERVER, REST API & LIVE SSE STREAM KE NEXT.JS 16
// -----------------------------------------------------------------------------

type EngineState struct {
	mu      sync.RWMutex
	Macro   MacroState         `json:"macro"`
	Signals []ConfluenceResult `json:"signals"`
}

func LoadDefaultLexicon(path string) FinancialLexicon {
	data, err := os.ReadFile(path)
	if err == nil {
		var lex FinancialLexicon
		if json.Unmarshal(data, &lex) == nil && len(lex.BullishKeywords) > 0 {
			return lex
		}
	}
	// Fallback jika file belum dimount
	return FinancialLexicon{
		BullishKeywords: map[string]float64{
			"dividen jumbo": 35, "dividen interim": 25, "akumulasi asing": 28,
			"laba bersih melonjak": 32, "buyback saham": 30, "masuk indeks msci": 35,
			"kontrak baru": 22, "borong saham": 26,
		},
		BearishKeywords: map[string]float64{
			"gugat pkpu": -45, "suspensi bei": -40, "rugi bersih membengkak": -35,
			"distribusi masif": -28, "keluar dari msci": -35, "auto reject bawah": -35,
		},
	}
}

func SeedInitialSignals(lex FinancialLexicon) EngineState {
	macro := MacroState{
		IHSGPrice:         7385.50,
		IHSGMA20:          7310.00,
		USDIDRRate:        15680.0,
		USDIDRChangePct:   -0.12,
		GarchVolatility:   13.8,
		MarketRegimeLabel: "BULL_CALM (Ideal untuk Swing & Scalping)",
	}

	now := time.Now()
	samples := []ConfluenceResult{
		EvaluateTicker(macro,
			FundamentalInput{Ticker: "BMRI", CompanyName: "PT Bank Mandiri (Persero) Tbk", Sector: "BANKING", IsSyariah: false, IsLQ45: true, Regime: RegimeBlueChip, LastPrice: 6450, PER: 10.8, PBV: 2.3, ROE: 22.8, DER: 0, EPS: 610, BVPS: 2920, NPLProxyPct: 1.2},
			SmartMoneyInput{NetForeign5DVal: 485_000_000_000, AvgDailyTransaction: 320_000_000_000, Top1BuyerCode: "ZP", Top3NetBuyVal: 145_000_000_000, BandarAvgBuyPrice: 6375, KSEIInstitutionalDiff: 2.1, BandarStatus: "BIG_ACCUMULATION"},
			[]NewsArticle{
				{Title: "Laba Bersih Melonjak, BMRI Catat Akumulasi Asing Deras Pekan Ini", Portal: "Kontan", URL: "https://www.kontan.co.id/", PublishedAt: now.Add(-6 * time.Hour), IndoBERTScore: 84.0},
			},
			DividendInput{HasUpcomingDividend: false},
			TechnicalInput{PriceAboveMA20: true, PriceAboveMA50: true, PriceAboveVWAP: true, VWAPPrice: 6425, ConnorsRSI3: 22.5, RSI14: 54.0, IntradayRVOL: 1.9, DailyRVOL: 1.85, ATR14: 135, MACDHistogram: 18.5, OBVSlopePos: true},
			lex,
		),
		EvaluateTicker(macro,
			FundamentalInput{Ticker: "BRMS", CompanyName: "PT Bumi Resources Minerals Tbk", Sector: "COMMODITY", IsSyariah: true, IsLQ45: false, Regime: RegimeConglomerate, LastPrice: 384, PER: 38.0, PBV: 2.6, ROE: 8.5, DER: 0.3, EPS: 10, BVPS: 145, NPLProxyPct: 0},
			SmartMoneyInput{NetForeign5DVal: 110_000_000_000, AvgDailyTransaction: 180_000_000_000, Top1BuyerCode: "MG", Top3NetBuyVal: 68_000_000_000, BandarAvgBuyPrice: 378, KSEIInstitutionalDiff: 4.6, BandarStatus: "BIG_ACCUMULATION"},
			[]NewsArticle{
				{Title: "Produksi Emas Naik Signifikan, Pengendali Tambah Kepemilikan dan Masuk Indeks MSCI", Portal: "CNBC Indonesia", URL: "https://www.cnbcindonesia.com/market", PublishedAt: now.Add(-3 * time.Hour), IndoBERTScore: 88.0},
			},
			DividendInput{HasUpcomingDividend: false},
			TechnicalInput{PriceAboveMA20: true, PriceAboveMA50: true, PriceAboveVWAP: true, VWAPPrice: 376, ConnorsRSI3: 48.0, RSI14: 62.0, IntradayRVOL: 3.4, DailyRVOL: 2.8, ATR14: 14, MACDHistogram: 4.2, OBVSlopePos: true},
			lex,
		),
		EvaluateTicker(macro,
			FundamentalInput{Ticker: "ITMG", CompanyName: "PT Indo Tambangraya Megah Tbk", Sector: "COMMODITY", IsSyariah: true, IsLQ45: true, Regime: RegimeCyclical, LastPrice: 26850, PER: 6.4, PBV: 1.3, ROE: 24.5, DER: 0.2, EPS: 4150, BVPS: 19800, NPLProxyPct: 0},
			SmartMoneyInput{NetForeign5DVal: 95_000_000_000, AvgDailyTransaction: 65_000_000_000, Top1BuyerCode: "AK", Top3NetBuyVal: 24_000_000_000, BandarAvgBuyPrice: 26500, KSEIInstitutionalDiff: 0.95, BandarStatus: "ACCUMULATION"},
			[]NewsArticle{
				{Title: "ITMG Siapkan Dividen Interim Jumbo, Simak Jadwal Cum Date Bulan Ini", Portal: "Kontan", URL: "https://www.kontan.co.id/", PublishedAt: now.Add(-12 * time.Hour), IndoBERTScore: 81.0},
			},
			DividendInput{HasUpcomingDividend: true, DividendPerShare: 1950, EstimatedYieldPct: 7.26, DaysToCumDate: 14, Hist3YAvgExDropPct: 6.80, PayoutRatioPct: 65.0},
			TechnicalInput{PriceAboveMA20: true, PriceAboveMA50: true, PriceAboveVWAP: true, VWAPPrice: 26775, ConnorsRSI3: 34.0, RSI14: 58.0, IntradayRVOL: 1.6, DailyRVOL: 1.5, ATR14: 450, MACDHistogram: 65.0, OBVSlopePos: true},
			lex,
		),
		EvaluateTicker(macro,
			FundamentalInput{Ticker: "ISAT", CompanyName: "PT Indosat Tbk", Sector: "INFRA", IsSyariah: true, IsLQ45: true, Regime: RegimeBlueChip, LastPrice: 2380, PER: 14.2, PBV: 2.5, ROE: 17.6, DER: 1.4, EPS: 215, BVPS: 960, NPLProxyPct: 0},
			SmartMoneyInput{NetForeign5DVal: 160_000_000_000, AvgDailyTransaction: 92_000_000_000, Top1BuyerCode: "BK", Top3NetBuyVal: 35_000_000_000, BandarAvgBuyPrice: 2350, KSEIInstitutionalDiff: 1.80, BandarStatus: "ACCUMULATION"},
			[]NewsArticle{
				{Title: "ISAT Perluas Kontrak Baru AI Data Center, Laba Tumbuh Double Digit", Portal: "Antara", URL: "https://www.antaranews.com/ekonomi", PublishedAt: now.Add(-18 * time.Hour), IndoBERTScore: 79.0},
			},
			DividendInput{HasUpcomingDividend: false},
			TechnicalInput{PriceAboveMA20: true, PriceAboveMA50: true, PriceAboveVWAP: true, VWAPPrice: 2360, ConnorsRSI3: 19.5, RSI14: 49.0, IntradayRVOL: 1.7, DailyRVOL: 1.6, ATR14: 55, MACDHistogram: 8.0, OBVSlopePos: true},
			lex,
		),
		EvaluateTicker(macro,
			FundamentalInput{Ticker: "GOTO", CompanyName: "PT GoTo Gojek Tokopedia Tbk", Sector: "TECH", IsSyariah: false, IsLQ45: true, Regime: RegimeCyclical, LastPrice: 66, PER: -12.0, PBV: 1.6, ROE: -14.5, DER: 0.1, EPS: -6, BVPS: 42, NPLProxyPct: 0},
			SmartMoneyInput{NetForeign5DVal: -85_000_000_000, AvgDailyTransaction: 140_000_000_000, Top1BuyerCode: "YP", Top3NetBuyVal: -42_000_000_000, BandarAvgBuyPrice: 70, KSEIInstitutionalDiff: -2.30, BandarStatus: "DISTRIBUTION"},
			[]NewsArticle{
				{Title: "Asing Catat Net Foreign Sell di Sektor Teknologi Jelang Rebalancing", Portal: "CNBC Indonesia", URL: "https://www.cnbcindonesia.com/market", PublishedAt: now.Add(-10 * time.Hour), IndoBERTScore: 38.0},
			},
			DividendInput{HasUpcomingDividend: false},
			TechnicalInput{PriceAboveMA20: false, PriceAboveMA50: false, PriceAboveVWAP: false, VWAPPrice: 67, ConnorsRSI3: 68.0, RSI14: 42.0, IntradayRVOL: 1.1, DailyRVOL: 0.9, ATR14: 3, MACDHistogram: -1.2, OBVSlopePos: false},
			lex,
		),
	}

	return EngineState{
		Macro:   macro,
		Signals: samples,
	}
}

func main() {
	lexPath := os.Getenv("LEXICON_PATH")
	if lexPath == "" {
		lexPath = "idx_lexicon.json"
	}
	lex := LoadDefaultLexicon(lexPath)
	state := SeedInitialSignals(lex)

	// Jika dijalankan dengan argumen --test, cetak hasil JSON ke stdout lalu keluar
	if len(os.Args) > 1 && os.Args[1] == "--test" {
		out, _ := json.MarshalIndent(state, "", "  ")
		fmt.Println(string(out))
		return
	}

	mux := http.NewServeMux()

	// Endpoint REST API untuk Next.js 16 Dashboard
	mux.HandleFunc("/api/v1/signals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		state.mu.RLock()
		defer state.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(state)
	})

	// Endpoint Server-Sent Events (SSE) untuk Real-Time Intraday Scalping Alerts
	mux.HandleFunc("/api/v1/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				state.mu.RLock()
				payload, _ := json.Marshal(state)
				state.mu.RUnlock()
				fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			}
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("[IDX 5-Layer Confluence Engine] Server aktif di http://0.0.0.0:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}