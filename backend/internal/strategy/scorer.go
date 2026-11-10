package strategy

import (
	"math"
	"time"

	"github.com/idx-trader/confluence-platform/internal/domain"
	"github.com/idx-trader/confluence-platform/internal/nlp"
)

// =============================================================================
// MONOREPO PATH: backend/internal/strategy/scorer.go
// Penggabung Skor Konfluensi 5 Lapis, Dividend Trap Shield, & Bootstrapper
// =============================================================================

func ComputeDividendStrategy(d domain.DividendInput, l2Fund, l3Bandar float64) (divScore float64, trapRisk float64, tactic string) {
	if !d.HasUpcomingDividend || d.EstimatedYieldPct <= 0 {
		return 20.0, 0.0, "NONE"
	}
	trapRisk = 30.0
	if d.Hist3YAvgExDropPct > d.EstimatedYieldPct {
		trapRisk += 35.0
	}
	if l3Bandar < 45.0 {
		trapRisk += 25.0
	}
	if d.DaysToCumDate <= 2 {
		trapRisk += 15.0
	}
	trapRisk = domain.ClampScore(trapRisk)

	score := 40.0
	if d.EstimatedYieldPct >= 6.0 {
		score += 25.0
	} else if d.EstimatedYieldPct >= 4.0 {
		score += 15.0
	}
	if d.DaysToCumDate >= 10 && d.DaysToCumDate <= 22 {
		score += 25.0
	} else if d.DaysToCumDate >= 4 && d.DaysToCumDate < 10 {
		score += 10.0
	} else if d.DaysToCumDate <= 3 {
		score -= 15.0
	}
	score += (l2Fund-50.0)*0.2 + (l3Bandar-50.0)*0.3
	divScore = domain.ClampScore(score)

	if trapRisk >= 60.0 {
		tactic = "PRE_CUM_EXIT (Jual Saat Cum-Date)"
	} else if d.EstimatedYieldPct >= 6.0 && l3Bandar >= 60.0 {
		tactic = "HOLD_OR_REBUY_EX (Ambil Dividen)"
	} else {
		tactic = "PRE_CUM_EXIT (Amankan Capital Gain)"
	}
	return divScore, trapRisk, tactic
}

func EvaluateTicker(macro domain.MacroState, f domain.FundamentalInput, sm domain.SmartMoneyInput, news []domain.NewsArticle, div domain.DividendInput, tech domain.TechnicalInput, lex nlp.FinancialLexicon) domain.ConfluenceResult {
	now := time.Now()
	l1 := ComputeLayer1Macro(macro)
	l2, fairVal, upside := ComputeLayer2Fundamental(f)
	l3 := ComputeLayer3SmartMoney(f, sm)
	enrichedNews, l4 := ComputeLayer4Sentiment(news, lex, now)
	return finalizeConfluenceResult(now, f, sm, tech, div, enrichedNews, fairVal, upside, l1, l2, l3, l4)
}

// EvaluateWithNLP mengevaluasi konfluensi 5 lapis menggunakan instance nlp.Engine langsung
func EvaluateWithNLP(macro domain.MacroState, f domain.FundamentalInput, sm domain.SmartMoneyInput, news []domain.NewsArticle, div domain.DividendInput, tech domain.TechnicalInput, engine *nlp.Engine) domain.ConfluenceResult {
	now := time.Now()
	l1 := ComputeLayer1Macro(macro)
	l2, fairVal, upside := ComputeLayer2Fundamental(f)
	l3 := ComputeLayer3SmartMoney(f, sm)
	var enrichedNews []domain.NewsArticle
	l4 := 50.0
	if engine != nil {
		enrichedNews, l4 = engine.ScoreArticles(news, now)
	} else {
		enrichedNews = news
	}
	return finalizeConfluenceResult(now, f, sm, tech, div, enrichedNews, fairVal, upside, l1, l2, l3, l4)
}

func finalizeConfluenceResult(now time.Time, f domain.FundamentalInput, sm domain.SmartMoneyInput, tech domain.TechnicalInput, div domain.DividendInput, enrichedNews []domain.NewsArticle, fairVal, upside, l1, l2, l3, l4 float64) domain.ConfluenceResult {
	l5Swing := ComputeLayer5Technical(tech, false)
	l5Scalp := ComputeLayer5Technical(tech, true)

	scalpingScore := domain.ClampScore(0.50*l5Scalp + 0.25*l4 + 0.15*l3 + 0.10*l1)
	swingScore := domain.ClampScore(0.35*l3 + 0.25*l5Swing + 0.20*l4 + 0.12*l2 + 0.08*l1)
	divScore, trapRisk, divTactic := ComputeDividendStrategy(div, l2, l3)

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

	atr := tech.ATR14
	if atr <= 0 {
		atr = f.LastPrice * 0.025
	}
	entryMin := domain.RoundToIDXTick(f.LastPrice - (0.35 * atr))
	entryMax := domain.RoundToIDXTick(f.LastPrice + (0.15 * atr))
	var sl, tp1, tp2 float64
	if bestMode == "SCALPING" {
		sl = domain.RoundToIDXTick(f.LastPrice - (1.1 * atr))
		tp1 = domain.RoundToIDXTick(f.LastPrice + (1.8 * atr))
		tp2 = domain.RoundToIDXTick(f.LastPrice + (3.0 * atr))
	} else {
		sl = domain.RoundToIDXTick(f.LastPrice - (1.8 * atr))
		tp1 = domain.RoundToIDXTick(f.LastPrice + (3.2 * atr))
		tp2 = domain.RoundToIDXTick(f.LastPrice + (5.5 * atr))
	}
	risk := math.Max(1.0, f.LastPrice-sl)
	rr := math.Round(((tp2-f.LastPrice)/risk)*100) / 100

	maxAlloc := 10.0
	if grade == "A+" {
		maxAlloc = 20.0
	} else if grade == "A" {
		maxAlloc = 15.0
	}
	if l1 < 45.0 {
		maxAlloc *= 0.5
	}

	return domain.ConfluenceResult{
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
		Plan: domain.TradePlan{
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

// BootstrapInitialSignals menginisialisasi data emiten BEI representatif saat server pertama kali menyala
func BootstrapInitialSignals(engine *nlp.Engine) (domain.MacroState, []domain.ConfluenceResult) {
	macro := domain.MacroState{
		IHSGPrice:         7385.50,
		IHSGMA20:          7310.00,
		USDIDRRate:        15680.0,
		USDIDRChangePct:   -0.12,
		GarchVolatility:   13.8,
		MarketRegimeLabel: "BULL_CALM (Ideal untuk Swing & Scalping)",
	}
	now := time.Now()
	signals := []domain.ConfluenceResult{
		EvaluateWithNLP(macro,
			domain.FundamentalInput{Ticker: "BMRI", CompanyName: "PT Bank Mandiri (Persero) Tbk", Sector: "BANKING", IsLQ45: true, Regime: domain.RegimeBlueChip, LastPrice: 6450, PER: 10.8, PBV: 2.3, ROE: 22.8, EPS: 610, BVPS: 2920, NPLProxyPct: 1.2},
			domain.SmartMoneyInput{NetForeign5DVal: 485_000_000_000, AvgDailyTransaction: 320_000_000_000, Top1BuyerCode: "ZP", Top3NetBuyVal: 145_000_000_000, BandarAvgBuyPrice: 6375, KSEIInstitutionalDiff: 2.1, BandarStatus: "BIG_ACCUMULATION"},
			[]domain.NewsArticle{{Title: "Laba Bersih Melonjak, BMRI Catat Akumulasi Asing Deras Pekan Ini", Portal: "Kontan", URL: "https://www.kontan.co.id/", PublishedAt: now.Add(-6 * time.Hour), IndoBERTScore: 84.0}},
			domain.DividendInput{HasUpcomingDividend: false},
			domain.TechnicalInput{PriceAboveMA20: true, PriceAboveMA50: true, PriceAboveVWAP: true, VWAPPrice: 6425, ConnorsRSI3: 22.5, RSI14: 54.0, IntradayRVOL: 1.9, DailyRVOL: 1.85, ATR14: 135, MACDHistogram: 18.5, OBVSlopePos: true},
			engine,
		),
		EvaluateWithNLP(macro,
			domain.FundamentalInput{Ticker: "BRMS", CompanyName: "PT Bumi Resources Minerals Tbk", Sector: "COMMODITY", IsSyariah: true, Regime: domain.RegimeConglomerate, LastPrice: 384, PER: 38.0, PBV: 2.6, ROE: 8.5, DER: 0.3, EPS: 10, BVPS: 145},
			domain.SmartMoneyInput{NetForeign5DVal: 110_000_000_000, AvgDailyTransaction: 180_000_000_000, Top1BuyerCode: "MG", Top3NetBuyVal: 68_000_000_000, BandarAvgBuyPrice: 378, KSEIInstitutionalDiff: 4.6, BandarStatus: "BIG_ACCUMULATION"},
			[]domain.NewsArticle{{Title: "Produksi Emas Naik Signifikan, Pengendali Tambah Kepemilikan dan Masuk Indeks MSCI", Portal: "CNBC Indonesia", URL: "https://www.cnbcindonesia.com/market", PublishedAt: now.Add(-3 * time.Hour), IndoBERTScore: 88.0}},
			domain.DividendInput{HasUpcomingDividend: false},
			domain.TechnicalInput{PriceAboveMA20: true, PriceAboveMA50: true, PriceAboveVWAP: true, VWAPPrice: 376, ConnorsRSI3: 48.0, RSI14: 62.0, IntradayRVOL: 3.4, DailyRVOL: 2.8, ATR14: 14, MACDHistogram: 4.2, OBVSlopePos: true},
			engine,
		),
		EvaluateWithNLP(macro,
			domain.FundamentalInput{Ticker: "ITMG", CompanyName: "PT Indo Tambangraya Megah Tbk", Sector: "COMMODITY", IsSyariah: true, IsLQ45: true, Regime: domain.RegimeCyclical, LastPrice: 26850, PER: 6.4, PBV: 1.3, ROE: 24.5, DER: 0.2, EPS: 4150, BVPS: 19800},
			domain.SmartMoneyInput{NetForeign5DVal: 95_000_000_000, AvgDailyTransaction: 65_000_000_000, Top1BuyerCode: "AK", Top3NetBuyVal: 24_000_000_000, BandarAvgBuyPrice: 26500, KSEIInstitutionalDiff: 0.95, BandarStatus: "ACCUMULATION"},
			[]domain.NewsArticle{{Title: "ITMG Siapkan Dividen Interim Jumbo, Simak Jadwal Cum Date Bulan Ini", Portal: "Kontan", URL: "https://www.kontan.co.id/", PublishedAt: now.Add(-12 * time.Hour), IndoBERTScore: 81.0}},
			domain.DividendInput{HasUpcomingDividend: true, DividendPerShare: 1950, EstimatedYieldPct: 7.26, DaysToCumDate: 14, Hist3YAvgExDropPct: 6.80, PayoutRatioPct: 65.0},
			domain.TechnicalInput{PriceAboveMA20: true, PriceAboveMA50: true, PriceAboveVWAP: true, VWAPPrice: 26775, ConnorsRSI3: 34.0, RSI14: 58.0, IntradayRVOL: 1.6, DailyRVOL: 1.5, ATR14: 450, MACDHistogram: 65.0, OBVSlopePos: true},
			engine,
		),
		EvaluateWithNLP(macro,
			domain.FundamentalInput{Ticker: "ISAT", CompanyName: "PT Indosat Tbk", Sector: "INFRA", IsSyariah: true, IsLQ45: true, Regime: domain.RegimeBlueChip, LastPrice: 2380, PER: 14.2, PBV: 2.5, ROE: 17.6, DER: 1.4, EPS: 215, BVPS: 960},
			domain.SmartMoneyInput{NetForeign5DVal: 160_000_000_000, AvgDailyTransaction: 92_000_000_000, Top1BuyerCode: "BK", Top3NetBuyVal: 35_000_000_000, BandarAvgBuyPrice: 2350, KSEIInstitutionalDiff: 1.80, BandarStatus: "ACCUMULATION"},
			[]domain.NewsArticle{{Title: "ISAT Perluas Kontrak Baru AI Data Center, Laba Tumbuh Double Digit", Portal: "Antara", URL: "https://www.antaranews.com/ekonomi", PublishedAt: now.Add(-18 * time.Hour), IndoBERTScore: 79.0}},
			domain.DividendInput{HasUpcomingDividend: false},
			domain.TechnicalInput{PriceAboveMA20: true, PriceAboveMA50: true, PriceAboveVWAP: true, VWAPPrice: 2360, ConnorsRSI3: 19.5, RSI14: 49.0, IntradayRVOL: 1.7, DailyRVOL: 1.6, ATR14: 55, MACDHistogram: 8.0, OBVSlopePos: true},
			engine,
		),
	}
	return macro, signals
}