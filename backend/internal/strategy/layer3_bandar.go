package strategy

import "github.com/idx-trader/confluence-platform/internal/domain"

// ComputeLayer3SmartMoney memisahkan rezim Foreign Flow (Blue-Chip) dan Broker Summary (Second/Third-Liner)
func ComputeLayer3SmartMoney(f domain.FundamentalInput, sm domain.SmartMoneyInput) float64 {
	score := 50.0
	if sm.AvgDailyTransaction <= 0 {
		return score
	}
	brokerAccumRatio := (sm.Top3NetBuyVal / sm.AvgDailyTransaction) * 100.0
	foreignFlowRatio := (sm.NetForeign5DVal / (sm.AvgDailyTransaction * 5.0)) * 100.0

	if f.Regime == domain.RegimeBlueChip || f.IsLQ45 {
		score += domain.ClampDelta(foreignFlowRatio*1.8, -28.0, 28.0)
		score += domain.ClampDelta(brokerAccumRatio*0.9, -15.0, 15.0)
	} else {
		score += domain.ClampDelta(brokerAccumRatio*1.7, -32.0, 32.0)
		score += domain.ClampDelta(foreignFlowRatio*0.7, -10.0, 10.0)
	}

	if sm.BandarAvgBuyPrice > 0 && f.LastPrice > 0 {
		diffPct := ((f.LastPrice - sm.BandarAvgBuyPrice) / sm.BandarAvgBuyPrice) * 100.0
		if diffPct >= -3.0 && diffPct <= 2.5 && sm.Top3NetBuyVal > 0 {
			score += 12.0
		} else if diffPct > 12.0 {
			score -= 10.0
		}
	}
	if sm.KSEIInstitutionalDiff > 0.5 {
		score += 6.0
	} else if sm.KSEIInstitutionalDiff < -1.0 {
		score -= 8.0
	}
	return domain.ClampScore(score)
}
