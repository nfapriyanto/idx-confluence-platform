package strategy

import "github.com/idx-trader/confluence-platform/internal/domain"

// ComputeLayer1Macro memeriksa tren IHSG vs MA20, fluktuasi USD/IDR, dan volatilitas GARCH
func ComputeLayer1Macro(m domain.MacroState) float64 {
	score := 50.0
	if m.IHSGPrice >= m.IHSGMA20 {
		score += 20.0
	} else {
		score -= 15.0
	}
	if m.USDIDRChangePct <= 0.15 {
		score += 15.0
	} else if m.USDIDRChangePct > 0.50 {
		score -= 20.0
	}
	if m.GarchVolatility < 16.0 {
		score += 15.0
	} else if m.GarchVolatility > 25.0 {
		score -= 25.0
	}
	return domain.ClampScore(score)
}
