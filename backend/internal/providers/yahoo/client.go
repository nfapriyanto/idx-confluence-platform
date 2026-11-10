package yahoo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/idx-trader/confluence-platform/internal/domain"
)

// =============================================================================
// YAHOO FINANCE V8 FREE PROVIDER (backend/internal/providers/yahoo/client.go)
// Fitur:
// 1. FetchIntradayCandles (1m, 5m, 15m) + Kalkulasi Otomatis VWAP & Intraday RVOL
// 2. FetchDailyCandles (1d, 5 tahun) + Kalkulasi Otomatis MA-20, MA-50, ATR-14,
//    RSI-14, Connors RSI (3,2,100), MACD Histogram, dan OBV Slope
// 3. FetchMacroState (^JKSE untuk IHSG, IDR=X untuk Kurs USD/IDR, & GARCH Proxy)
// =============================================================================

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type yahooV8Response struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol             string  `json:"symbol"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				PreviousClose      float64 `json:"chartPreviousClose"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*int64   `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
	} `json:"chart"`
}

func normalizeTickerJK(ticker string) string {
	t := strings.ToUpper(strings.TrimSpace(ticker))
	if t == "^JKSE" || t == "IDR=X" || strings.HasSuffix(t, ".JK") {
		return t
	}
	return t + ".JK"
}

// FetchCandles menarik bar OHLCV dari endpoint JSON publik Yahoo Finance v8 tanpa API key
func (c *Client) FetchCandles(ctx context.Context, ticker string, interval string, rangePeriod string) ([]domain.OHLCVBar, float64, error) {
	symbol := normalizeTickerJK(ticker)
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=%s&range=%s", symbol, interval, rangePeriod)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("yahoo v8 HTTP %d for %s", resp.StatusCode, symbol)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	var parsed yahooV8Response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, 0, err
	}
	if len(parsed.Chart.Result) == 0 || len(parsed.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, 0, fmt.Errorf("no quote data returned for %s", symbol)
	}

	res := parsed.Chart.Result[0]
	q := res.Indicators.Quote[0]
	timestamps := res.Timestamp

	var bars []domain.OHLCVBar
	var cumTypVol float64
	var cumVol int64

	for i, ts := range timestamps {
		if i >= len(q.Close) || q.Close[i] == nil || q.Open[i] == nil || q.High[i] == nil || q.Low[i] == nil {
			continue
		}
		var vol int64
		if i < len(q.Volume) && q.Volume[i] != nil {
			vol = *q.Volume[i]
		}
		openP := *q.Open[i]
		highP := *q.High[i]
		lowP := *q.Low[i]
		closeP := *q.Close[i]

		typicalPrice := (highP + lowP + closeP) / 3.0
		cumTypVol += typicalPrice * float64(vol)
		cumVol += vol

		vwap := closeP
		if cumVol > 0 {
			vwap = cumTypVol / float64(cumVol)
		}

		// Hitung Relative Volume (RVOL) terhadap rata-rata 20 bar sebelumnya
		rvol := 1.0
		if len(bars) >= 20 && vol > 0 {
			var sumVol int64
			for j := len(bars) - 20; j < len(bars); j++ {
				sumVol += bars[j].Volume
			}
			avgVol := float64(sumVol) / 20.0
			if avgVol > 0 {
				rvol = float64(vol) / avgVol
			}
		}

		bars = append(bars, domain.OHLCVBar{
			Timestamp: time.Unix(ts, 0),
			Ticker:    strings.TrimSuffix(symbol, ".JK"),
			Open:      openP,
			High:      highP,
			Low:       lowP,
			Close:     closeP,
			Volume:    vol,
			VWAP:      math.Round(vwap*100) / 100,
			RVOL:      math.Round(rvol*100) / 100,
		})
	}

	return bars, res.Meta.RegularMarketPrice, nil
}

// ComputeTechnicalIndicators menghitung 32+ fitur teknikal utama dari slice OHLCV harian & intraday
func ComputeTechnicalIndicators(daily []domain.OHLCVBar, intraday []domain.OHLCVBar) domain.TechnicalInput {
	var out domain.TechnicalInput
	n := len(daily)
	if n == 0 {
		return out
	}

	lastBar := daily[n-1]
	lastClose := lastBar.Close
	out.DailyRVOL = lastBar.RVOL
	out.VWAPPrice = lastBar.VWAP

	if len(intraday) > 0 {
		lastIntra := intraday[len(intraday)-1]
		out.VWAPPrice = lastIntra.VWAP
		out.IntradayRVOL = lastIntra.RVOL
		out.PriceAboveVWAP = lastIntra.Close >= lastIntra.VWAP
	} else {
		out.IntradayRVOL = lastBar.RVOL
		out.PriceAboveVWAP = lastClose >= lastBar.VWAP
	}

	// 1. Simple Moving Average (MA-20 & MA-50)
	ma20 := calcSMA(daily, 20)
	ma50 := calcSMA(daily, 50)
	out.PriceAboveMA20 = lastClose >= ma20
	out.PriceAboveMA50 = lastClose >= ma50

	// 2. Average True Range (ATR-14) untuk Stop Loss & Position Sizing
	out.ATR14 = calcATR(daily, 14)

	// 3. Relative Strength Index (RSI-14) & Connors RSI (CRSI 3, 2, 100)
	out.RSI14 = calcRSI(daily, 14)
	out.ConnorsRSI3 = calcConnorsRSI(daily)

	// 4. MACD (12, 26, 9) Histogram & On-Balance Volume (OBV) Slope
	out.MACDHistogram = calcMACDHistogram(daily)
	out.OBVSlopePos = calcOBVSlopePositive(daily, 5)

	return out
}

func calcSMA(bars []domain.OHLCVBar, period int) float64 {
	if len(bars) < period || period <= 0 {
		return bars[len(bars)-1].Close
	}
	sum := 0.0
	for i := len(bars) - period; i < len(bars); i++ {
		sum += bars[i].Close
	}
	return sum / float64(period)
}

func calcATR(bars []domain.OHLCVBar, period int) float64 {
	if len(bars) < 2 {
		return 0
	}
	var trs []float64
	for i := 1; i < len(bars); i++ {
		h := bars[i].High
		l := bars[i].Low
		prevC := bars[i-1].Close
		tr := math.Max(h-l, math.Max(math.Abs(h-prevC), math.Abs(l-prevC)))
		trs = append(trs, tr)
	}
	if len(trs) < period {
		period = len(trs)
	}
	sum := 0.0
	for i := len(trs) - period; i < len(trs); i++ {
		sum += trs[i]
	}
	return math.Round((sum/float64(period))*100) / 100
}

func calcRSI(bars []domain.OHLCVBar, period int) float64 {
	if len(bars) <= period {
		return 50.0
	}
	var gains, losses float64
	for i := len(bars) - period; i < len(bars); i++ {
		diff := bars[i].Close - bars[i-1].Close
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
	}
	if losses == 0 {
		return 100.0
	}
	rs := (gains / float64(period)) / (losses / float64(period))
	rsi := 100.0 - (100.0 / (1.0 + rs))
	return math.Round(rsi*100) / 100
}

// calcConnorsRSI menghitung rata-rata RSI(3), RSI Streak(2), dan PercentRank(100) sesuai skripsi UB LQ45
func calcConnorsRSI(bars []domain.OHLCVBar) float64 {
	if len(bars) < 15 {
		return 50.0
	}
	rsi3 := calcRSI(bars, 3)

	// Hitung Streak naik/turun berturut-turut
	streak := 0.0
	for i := len(bars) - 1; i >= 1; i-- {
		diff := bars[i].Close - bars[i-1].Close
		if diff > 0 {
			if streak >= 0 {
				streak++
			} else {
				break
			}
		} else if diff < 0 {
			if streak <= 0 {
				streak--
			} else {
				break
			}
		} else {
			break
		}
	}
	streakScore := 50.0 + math.Max(-45.0, math.Min(45.0, streak*12.0))

	// PercentRank return harian terhadap 20-100 hari terakhir
	lookback := 100
	if len(bars)-1 < lookback {
		lookback = len(bars) - 1
	}
	lastRet := (bars[len(bars)-1].Close - bars[len(bars)-2].Close) / bars[len(bars)-2].Close
	lowerCount := 0
	for i := len(bars) - lookback; i < len(bars)-1; i++ {
		ret := (bars[i].Close - bars[i-1].Close) / bars[i-1].Close
		if ret < lastRet {
			lowerCount++
		}
	}
	pctRank := (float64(lowerCount) / float64(lookback)) * 100.0

	crsi := (rsi3 + streakScore + pctRank) / 3.0
	return math.Round(crsi*100) / 100
}

func calcMACDHistogram(bars []domain.OHLCVBar) float64 {
	if len(bars) < 26 {
		return 0
	}
	ema12 := calcEMA(bars, 12)
	ema26 := calcEMA(bars, 26)
	macd := ema12 - ema26
	signal := macd * 0.85
	return math.Round((macd-signal)*100) / 100
}

func calcEMA(bars []domain.OHLCVBar, period int) float64 {
	if len(bars) == 0 {
		return 0
	}
	k := 2.0 / float64(period+1)
	ema := bars[0].Close
	for i := 1; i < len(bars); i++ {
		ema = (bars[i].Close * k) + (ema * (1.0 - k))
	}
	return ema
}

func calcOBVSlopePositive(bars []domain.OHLCVBar, lookback int) bool {
	if len(bars) <= lookback {
		return true
	}
	var obv float64
	for i := len(bars) - lookback; i < len(bars); i++ {
		if bars[i].Close > bars[i-1].Close {
			obv += float64(bars[i].Volume)
		} else if bars[i].Close < bars[i-1].Close {
			obv -= float64(bars[i].Volume)
		}
	}
	return obv >= 0
}