package strategy

import (
	"math"

	"github.com/idx-trader/confluence-platform/internal/domain"
)

// ComputeLayer2Fundamental menghitung nilai wajar sektoral, Quality Penalty, dan Upside Bonus
func ComputeLayer2Fundamental(f domain.FundamentalInput) (score float64, fairVal float64, upsidePct float64) {
	baseScore := 50.0
	switch f.Sector {
	case "BANKING":
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
			fairVal = f.EPS * 8.5
		} else {
			fairVal = f.BVPS * 1.2
		}
	default:
		if f.EPS > 0 {
			fairVal = f.EPS * 17.5
		} else {
			fairVal = f.BVPS * 2.5
		}
	}

	if f.LastPrice > 0 && fairVal > 0 {
		upsidePct = ((fairVal - f.LastPrice) / f.LastPrice) * 100.0
	}
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

	upsideBonus := 0.0
	if upsidePct > 20.0 {
		upsideBonus = math.Min(upsidePct/10.0, 10.0)
	}

	finalScore := domain.ClampScore(baseScore - qualityPenalty + upsideBonus)
	return finalScore, domain.RoundToIDXTick(fairVal), math.Round(upsidePct*100) / 100
}
