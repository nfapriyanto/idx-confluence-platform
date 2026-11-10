package nlp

import (
	"encoding/json"
	"os"
)

type FinancialLexicon struct {
	BullishKeywords map[string]float64 `json:"bullish_keywords"`
	BearishKeywords map[string]float64 `json:"bearish_keywords"`
}

func LoadLexicon(path string) FinancialLexicon {
	data, err := os.ReadFile(path)
	if err == nil {
		var lex FinancialLexicon
		if json.Unmarshal(data, &lex) == nil && len(lex.BullishKeywords) > 0 {
			return lex
		}
	}
	return FinancialLexicon{
		BullishKeywords: map[string]float64{
			"dividen jumbo": 35, "dividen interim": 25, "akumulasi asing": 28,
			"laba bersih melonjak": 32, "buyback saham": 30, "masuk indeks msci": 35,
			"kontrak baru": 22, "borong saham": 26, "pengendali tambah kepemilikan": 30,
		},
		BearishKeywords: map[string]float64{
			"gugat pkpu": -45, "suspensi bei": -40, "rugi bersih membengkak": -35,
			"distribusi masif": -28, "keluar dari msci": -35, "auto reject bawah": -35,
		},
	}
}
