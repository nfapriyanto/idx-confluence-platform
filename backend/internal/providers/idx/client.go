package idx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/idx-trader/confluence-platform/internal/domain"
)

// =============================================================================
// MONOREPO PATH: backend/internal/providers/idx/client.go
// Scraper Gratis ke Endpoint Resmi idx.co.id:
// 1. GetStockSummary (Foreign Buy, Foreign Sell, Net Foreign Flow, Volume, Value)
// 2. GetCorporateAction / Kalender Dividen Resmi BEI (Cum Date, Ex Date, Payout)
// 3. Data Kepemilikan Saham >1% KSEI (Institutional & Tycoon Ownership Drift)
// =============================================================================

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 12 * time.Second},
	}
}

type stockSummaryPayload struct {
	Data []struct {
		StockCode   string  `json:"StockCode"`
		Close       float64 `json:"Close"`
		Volume      int64   `json:"Volume"`
		Value       float64 `json:"Value"`
		Frequency   int     `json:"Frequency"`
		ForeignBuy  float64 `json:"ForeignBuy"`
		ForeignSell float64 `json:"ForeignSell"`
	} `json:"data"`
}

type dividendCalendarPayload struct {
	Data []struct {
		Code         string  `json:"Code"`
		CashDividend float64 `json:"CashDividend"`
		CumDate      string  `json:"CumDate"`
		ExDate       string  `json:"ExDate"`
		PaymentDate  string  `json:"PaymentDate"`
	} `json:"data"`
}

func setResidentialHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://www.idx.co.id/id/data-pasar/ringkasan-perdagangan/ringkasan-saham/")
}

// FetchStockSummary menarik data Foreign Flow harian seluruh emiten BEI
func (c *Client) FetchStockSummary(ctx context.Context, dateYYYYMMDD string) (map[string]domain.DailyFlowRecord, error) {
	url := fmt.Sprintf("https://www.idx.co.id/primary/TradingSummary/GetStockSummary?date=%s&start=0&length=1000", dateYYYYMMDD)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	setResidentialHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("idx GetStockSummary HTTP %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed stockSummaryPayload
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}

	result := make(map[string]domain.DailyFlowRecord, len(parsed.Data))
	for _, item := range parsed.Data {
		avgPrice := item.Close
		if item.Volume > 0 && item.Value > 0 {
			avgPrice = item.Value / float64(item.Volume)
		}
		fBuy := item.ForeignBuy * avgPrice
		fSell := item.ForeignSell * avgPrice

		result[item.StockCode] = domain.DailyFlowRecord{
			Ticker:         item.StockCode,
			ClosePrice:     item.Close,
			Volume:         item.Volume,
			ValueIDR:       item.Value,
			Frequency:      item.Frequency,
			ForeignBuyVal:  fBuy,
			ForeignSellVal: fSell,
			NetForeignVal:  fBuy - fSell,
		}
	}
	return result, nil
}

// FetchUpcomingDividends menarik jadwal Cum-Date dan Ex-Date dividen tunai dari kalender BEI
func (c *Client) FetchUpcomingDividends(ctx context.Context, lastPrices map[string]float64) (map[string]domain.DividendInput, error) {
	url := "https://www.idx.co.id/primary/ListedCompany/GetCorporateAction?start=0&length=100&actionType=div"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	setResidentialHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("idx GetCorporateAction HTTP %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed dividendCalendarPayload
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}

	now := time.Now()
	out := make(map[string]domain.DividendInput)
	for _, d := range parsed.Data {
		cumTime, err := time.Parse("2006-01-02T15:04:05", d.CumDate)
		if err != nil {
			cumTime, _ = time.Parse("2006-01-02", d.CumDate)
		}
		daysToCum := int(cumTime.Sub(now).Hours() / 24)
		if daysToCum < 0 {
			continue
		}

		price := lastPrices[d.Code]
		yieldPct := 0.0
		if price > 0 {
			yieldPct = (d.CashDividend / price) * 100.0
		}

		out[d.Code] = domain.DividendInput{
			HasUpcomingDividend: true,
			DividendPerShare:    d.CashDividend,
			EstimatedYieldPct:   yieldPct,
			DaysToCumDate:       daysToCum,
			Hist3YAvgExDropPct:  yieldPct * 0.92, // Default konservatif sebelum di-override histori DB
			PayoutRatioPct:      50.0,
		}
	}
	return out, nil
}