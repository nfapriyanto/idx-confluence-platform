package strategy

import "github.com/idx-trader/confluence-platform/internal/domain"

// ComputeLayer5Technical mengevaluasi RVOL, VWAP, Connors RSI, MA20/50, MACD, dan OBV
func ComputeLayer5Technical(t domain.TechnicalInput, isScalpingMode bool) float64 {
	score := 45.0
	if isScalpingMode {
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
			score -= 15.0
		}
	} else {
		if t.PriceAboveMA20 {
			score += 12.0
		}
		if t.PriceAboveMA50 {
			score += 10.0
		}
		if t.PriceAboveMA50 && t.ConnorsRSI3 < 25.0 {
			score += 18.0
		} else if t.PriceAboveMA20 && t.DailyRVOL >= 1.8 {
			score += 15.0
		}
		if t.MACDHistogram > 0 {
			score += 8.0
		}
		if t.OBVSlopePos {
			score += 7.0
		}
	}
	return domain.ClampScore(score)
}
