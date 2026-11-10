package strategy

import (
	"math"
	"time"

	"github.com/idx-trader/confluence-platform/internal/domain"
	"github.com/idx-trader/confluence-platform/internal/nlp"
)

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
