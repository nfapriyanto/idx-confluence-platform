package strategy

import (
	"math"
	"strings"
	"time"

	"github.com/idx-trader/confluence-platform/internal/domain"
	"github.com/idx-trader/confluence-platform/internal/nlp"
)

// ComputeLayer4Sentiment menggabungkan 60% IndoBERT + 40% Financial Lexicon dengan peluruhan 3 hari
func ComputeLayer4Sentiment(articles []domain.NewsArticle, lex nlp.FinancialLexicon, now time.Time) ([]domain.NewsArticle, float64) {
	if len(articles) == 0 {
		return articles, 50.0
	}
	const halfLifeHours = 72.0
	lambda := math.Ln2 / halfLifeHours

	var weightedSum, totalWeight float64
	enriched := make([]domain.NewsArticle, len(articles))

	for i, art := range articles {
		ageHours := math.Max(0, now.Sub(art.PublishedAt).Hours())
		weight := math.Exp(-lambda * ageHours)

		lowerTitle := strings.ToLower(art.Title)
		lexDelta := 0.0
		for kw, pts := range lex.BullishKeywords {
			if strings.Contains(lowerTitle, kw) {
				lexDelta += pts
			}
		}
		for kw, pts := range lex.BearishKeywords {
			if strings.Contains(lowerTitle, kw) {
				lexDelta += pts
			}
		}
		lexScore := domain.ClampScore(50.0 + lexDelta)
		hybrid := (0.60 * art.IndoBERTScore) + (0.40 * lexScore)

		art.LexiconBonus = lexDelta
		art.HybridScore = domain.ClampScore(hybrid)
		art.AgeHours = math.Round(ageHours*10) / 10
		art.DecayedWeight = math.Round(weight*1000) / 1000
		enriched[i] = art

		weightedSum += art.HybridScore * weight
		totalWeight += weight
	}
	if totalWeight == 0 {
		return enriched, 50.0
	}
	return enriched, domain.ClampScore(weightedSum / totalWeight)
}
