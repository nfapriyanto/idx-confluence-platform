package ipot

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/idx-trader/confluence-platform/internal/domain"
)

// =============================================================================
// MONOREPO PATH: backend/internal/providers/ipot/client.go
// Scraper Gratis Ringkasan Broker (Bandarmology) Indo Premier (IPOT):
// - Dieksekusi setiap hari bursa pukul 16:35 WIB setelah kode broker dibuka BEI
// - Menghitung Top 1 Buyer, Top 3 Net Buy Accumulation Ratio, serta
//   Bandar Average Buy Price (Harga Modal Rata-Rata Akumulasi Bandar)
// =============================================================================

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type BrokerRow struct {
	BrokerCode string  `json:"broker_code"`
	BuyVol     int64   `json:"buy_vol"`
	BuyVal     float64 `json:"buy_val"`
	BuyAvg     float64 `json:"buy_avg"`
	SellVol    int64   `json:"sell_vol"`
	SellVal    float64 `json:"sell_val"`
	SellAvg    float64 `json:"sell_avg"`
	NetVal     float64 `json:"net_val"`
}

var rowPattern = regexp.MustCompile(`(?s)<tr[^>]*>.*?<td[^>]*>([A-Z]{2})</td>.*?<td[^>]*>([0-9.,-]+)</td>.*?<td[^>]*>([0-9.,]+)</td>`)

// FetchBrokerSummary menarik halaman publik Broker Summary Indo Premier untuk saham & tanggal tertentu
func (c *Client) FetchBrokerSummary(ctx context.Context, ticker string, dateYYYYMMDD string, totalDailyVal float64) (domain.SmartMoneyInput, error) {
	url := fmt.Sprintf("https://www.indopremier.com/ipotnews/newsSmartSearch.php?code=%s&date=%s", strings.ToUpper(ticker), dateYYYYMMDD)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.SmartMoneyInput{BandarStatus: "NEUTRAL"}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.SmartMoneyInput{BandarStatus: "NEUTRAL"}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.SmartMoneyInput{BandarStatus: "NEUTRAL"}, err
	}

	return ParseBrokerTable(string(body), totalDailyVal), nil
}

// ParseBrokerTable mengolah tabel transaksi broker menjadi indikator Akumulasi / Distribusi Bandar
func ParseBrokerTable(htmlStr string, totalTransactionVal float64) domain.SmartMoneyInput {
	matches := rowPattern.FindAllStringSubmatch(htmlStr, -1)
	if len(matches) == 0 || totalTransactionVal <= 0 {
		return domain.SmartMoneyInput{
			AvgDailyTransaction: totalTransactionVal,
			BandarStatus:        "NEUTRAL",
		}
	}

	var brokers []BrokerRow
	for _, m := range matches {
		code := strings.TrimSpace(m[1])
		netVal := parseNumber(m[2])
		avgPrice := parseNumber(m[3])
		brokers = append(brokers, BrokerRow{
			BrokerCode: code,
			BuyVal:     math.Max(0, netVal),
			BuyAvg:     avgPrice,
			NetVal:     netVal,
		})
	}

	sort.Slice(brokers, func(i, j int) bool {
		return brokers[i].NetVal > brokers[j].NetVal
	})

	top1Code := brokers[0].BrokerCode
	var top3NetBuy, top3BuyValSum, top3BuyShares float64

	for i, b := range brokers {
		if i < 3 {
			top3NetBuy += b.NetVal
			if b.BuyAvg > 0 && b.BuyVal > 0 {
				top3BuyValSum += b.BuyVal
				top3BuyShares += b.BuyVal / b.BuyAvg
			}
		}
	}

	bandarAvg := 0.0
	if top3BuyShares > 0 {
		bandarAvg = domain.RoundToIDXTick(top3BuyValSum / top3BuyShares)
	}

	accumRatioPct := (top3NetBuy / totalTransactionVal) * 100.0
	status := "NEUTRAL"
	switch {
	case accumRatioPct >= 18.0:
		status = "BIG_ACCUMULATION"
	case accumRatioPct >= 8.0:
		status = "ACCUMULATION"
	case accumRatioPct <= -18.0:
		status = "BIG_DISTRIBUTION"
	case accumRatioPct <= -8.0:
		status = "DISTRIBUTION"
	}

	return domain.SmartMoneyInput{
		AvgDailyTransaction: totalTransactionVal,
		Top1BuyerCode:       top1Code,
		Top3NetBuyVal:       math.Round(top3NetBuy),
		BandarAvgBuyPrice:   bandarAvg,
		BandarStatus:        status,
	}
}

func parseNumber(s string) float64 {
	clean := strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	v, _ := strconv.ParseFloat(clean, 64)
	return v
}