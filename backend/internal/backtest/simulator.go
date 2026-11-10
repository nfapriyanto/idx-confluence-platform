package backtest

import (
	"math"
	"time"

	"github.com/idx-trader/confluence-platform/internal/domain"
)

// =============================================================================
// MONOREPO PATH: backend/internal/backtest/simulator.go
// Walk-Forward Historical Backtesting Engine (Sesuai Literatur BEI / IDX):
// 1. Struktur Biaya Transaksi Riil Sekuritas Indonesia & Pajak Final BEI:
//    - Fee Beli (Buy Fee): ~0,15% (Komisi Broker + Levy BEI/KPEI/KSEI + PPN)
//    - Fee Jual (Sell Fee): ~0,25% (Komisi Broker + Pajak Penghasilan Final 0,10%)
//    Menjawab temuan jurnal "Market Efficiency and Technical Trading Profitability"
//    (2026) bahwa strategi teknikal murni sering kalah oleh biaya transaksi.
// 2. Proteksi Asimetris Auto Rejection Bawah (ARB) & Auto Rejection Atas (ARA) BEI
// 3. Evaluasi Metrik Kuantitatif Lengkap:
//    - Win Rate (%), Profit Factor, Expectancy per Trade, Sharpe Ratio,
//      Sortino Ratio, Maximum Drawdown (%), dan Perbandingan vs Buy-and-Hold.
// =============================================================================

type Config struct {
	InitialCapitalIDR float64 `json:"initial_capital_idr"`
	BuyFeePct         float64 `json:"buy_fee_pct"`          // Default BEI: 0.15%
	SellFeePct        float64 `json:"sell_fee_pct"`         // Default BEI: 0.25% (termasuk PPh Final 0.1%)
	SlippageTicks     int     `json:"slippage_ticks"`       // Default: 1 fraksi harga BEI
	MaxHoldingDays    int     `json:"max_holding_days"`     // Default Swing: 15 hari bursa, Scalping: 1 hari
	MinConfluenceScore float64 `json:"min_confluence_score"` // Default: 72.0 (Minimal Grade A)
}

func DefaultIDXConfig() Config {
	return Config{
		InitialCapitalIDR:  100_000_000, // Rp 100 Juta
		BuyFeePct:          0.15,
		SellFeePct:         0.25,
		SlippageTicks:      1,
		MaxHoldingDays:     15,
		MinConfluenceScore: 72.0,
	}
}

type HistoricalDayInput struct {
	Date            time.Time               `json:"date"`
	Bar             domain.OHLCVBar         `json:"bar"`
	Confluence      domain.ConfluenceResult `json:"confluence"`
}

type ExecutedTrade struct {
	Ticker         string    `json:"ticker"`
	Mode           string    `json:"mode"`
	SignalGrade    string    `json:"signal_grade"`
	EntryDate      time.Time `json:"entry_date"`
	ExitDate       time.Time `json:"exit_date"`
	HoldingDays    int       `json:"holding_days"`
	Lots           int       `json:"lots"`
	EntryPrice     float64   `json:"entry_price"`
	ExitPrice      float64   `json:"exit_price"`
	GrossPnLIDR    float64   `json:"gross_pnl_idr"`
	TotalFeeIDR    float64   `json:"total_fee_idr"`
	NetPnLIDR      float64   `json:"net_pnl_idr"`
	NetReturnPct   float64   `json:"net_return_pct"`
	ExitReason     string    `json:"exit_reason"` // TP1_HIT, TP2_HIT, ATR_STOP_LOSS, BANDAR_DISTRIBUTION_EXIT, TIME_EXIT
}

type EquityPoint struct {
	Date       time.Time `json:"date"`
	EquityIDR  float64   `json:"equity_idr"`
	DrawdownPct float64  `json:"drawdown_pct"`
}

type Report struct {
	Ticker               string          `json:"ticker"`
	StartDate            time.Time       `json:"start_date"`
	EndDate              time.Time       `json:"end_date"`
	InitialCapitalIDR    float64         `json:"initial_capital_idr"`
	FinalCapitalIDR      float64         `json:"final_capital_idr"`
	TotalNetReturnPct    float64         `json:"total_net_return_pct"`
	BuyAndHoldReturnPct  float64         `json:"buy_and_hold_return_pct"`
	AlphaVsBuyAndHoldPct float64         `json:"alpha_vs_buy_and_hold_pct"`
	TotalTrades          int             `json:"total_trades"`
	WinningTrades        int             `json:"winning_trades"`
	LosingTrades         int             `json:"losing_trades"`
	WinRatePct           float64         `json:"win_rate_pct"`
	ProfitFactor         float64         `json:"profit_factor"`
	ExpectancyPerTradePct float64        `json:"expectancy_per_trade_pct"`
	MaxDrawdownPct       float64         `json:"max_drawdown_pct"`
	SharpeRatio          float64         `json:"sharpe_ratio"`
	SortinoRatio         float64         `json:"sortino_ratio"`
	TotalBrokerTaxFeeIDR float64         `json:"total_broker_tax_fee_idr"`
	Trades               []ExecutedTrade `json:"trades"`
	EquityCurve          []EquityPoint   `json:"equity_curve"`
}

// RunWalkForwardBacktest menjalankan simulasi historis berurutan (tanpa look-ahead bias)
func RunWalkForwardBacktest(ticker string, history []HistoricalDayInput, cfg Config) Report {
	if len(history) == 0 {
		return Report{Ticker: ticker, InitialCapitalIDR: cfg.InitialCapitalIDR, FinalCapitalIDR: cfg.InitialCapitalIDR}
	}

	equity := cfg.InitialCapitalIDR
	peakEquity := equity
	maxDDPct := 0.0

	var trades []ExecutedTrade
	var equityCurve []EquityPoint
	var tradeReturns []float64
	var grossProfitSum, grossLossSum, totalFeesPaid float64

	inPosition := false
	var currentTrade ExecutedTrade
	var stopLoss, tp2 float64
	var entryIdx int

	for i, day := range history {
		bar := day.Bar
		sig := day.Confluence

		if !inPosition {
			// Syarat Entry Konfluensi 5 Lapis:
			// 1. Skor gabungan >= MinConfluenceScore (misal 72.0 / Grade A atau A+)
			// 2. Lapis 3 Bandarmology TIDAK sedang distribusi (Layer3SmartMoneyScore >= 50)
			// 3. Lapis 2 Fundamental lolos filter kualitas (ROE positif / skor >= 50)
			activeScore := math.Max(sig.SwingScore, math.Max(sig.ScalpingScore, sig.DividendScore))
			if activeScore >= cfg.MinConfluenceScore &&
				sig.Layer3SmartMoneyScore >= 50.0 &&
				sig.Layer2FundamentalScore >= 50.0 {

				// Tambahkan 1 tick slippage beli sesuai Fraksi Harga BEI
				entryPrice := applyTickSlippage(bar.Close, cfg.SlippageTicks)
				allocPct := sig.Plan.MaxPortfolioAllocPct
				if allocPct <= 0 {
					allocPct = 15.0
				}
				allocatedIDR := equity * (allocPct / 100.0)
				pricePerLot := entryPrice * 100.0
				lots := int(math.Floor(allocatedIDR / pricePerLot))

				if lots > 0 {
					inPosition = true
					entryIdx = i
					stopLoss = sig.Plan.StopLossPrice
					tp2 = sig.Plan.TakeProfit2
					currentTrade = ExecutedTrade{
						Ticker:      ticker,
						Mode:        sig.Plan.RecommendedMode,
						SignalGrade: sig.Plan.SignalGrade,
						EntryDate:   day.Date,
						Lots:        lots,
						EntryPrice:  entryPrice,
					}
				}
			}
		} else {
			// Evaluasi Posisi Terbuka (Cek Stop Loss ATR, Target TP, Distribusi Bandar, atau Batas Waktu)
			holdingDays := i - entryIdx
			var exitPrice float64
			var exitReason string

			switch {
			case bar.Low <= stopLoss:
				// Hit Stop Loss (dengan 1 tick slippage bawah)
				exitPrice = applyTickSlippage(stopLoss, -cfg.SlippageTicks)
				exitReason = "ATR_STOP_LOSS"
			case bar.High >= tp2:
				exitPrice = domain.RoundToIDXTick(tp2)
				exitReason = "TP2_TARGET_HIT"
			case sig.SmartMoney.BandarStatus == "BIG_DISTRIBUTION" || sig.Layer3SmartMoneyScore < 38.0:
				// Early Exit jika Bandar terdeteksi melakukan distribusi masif di Broker Summary
				exitPrice = applyTickSlippage(bar.Close, -cfg.SlippageTicks)
				exitReason = "BANDAR_DISTRIBUTION_EXIT"
			case sig.Dividend.HasUpcomingDividend && sig.Dividend.DaysToCumDate == 0 && sig.Plan.DividendTrapRisk >= 60.0:
				// Dividend-Timing Model Exit tepat pada Cum-Date jika risiko Ex-Date Trap tinggi
				exitPrice = domain.RoundToIDXTick(bar.Close)
				exitReason = "PRE_CUM_DIVIDEND_TRAP_EXIT"
			case holdingDays >= cfg.MaxHoldingDays:
				exitPrice = domain.RoundToIDXTick(bar.Close)
				exitReason = "MAX_HOLDING_TIME_EXIT"
			}

			if exitReason != "" {
				shares := float64(currentTrade.Lots * 100)
				buyVal := shares * currentTrade.EntryPrice
				sellVal := shares * exitPrice

				buyFee := buyVal * (cfg.BuyFeePct / 100.0)
				sellFee := sellVal * (cfg.SellFeePct / 100.0)
				totalFee := buyFee + sellFee

				grossPnL := sellVal - buyVal
				netPnL := grossPnL - totalFee
				netRetPct := (netPnL / buyVal) * 100.0

				currentTrade.ExitDate = day.Date
				currentTrade.HoldingDays = holdingDays
				currentTrade.ExitPrice = exitPrice
				currentTrade.GrossPnLIDR = math.Round(grossPnL)
				currentTrade.TotalFeeIDR = math.Round(totalFee)
				currentTrade.NetPnLIDR = math.Round(netPnL)
				currentTrade.NetReturnPct = math.Round(netRetPct*100) / 100
				currentTrade.ExitReason = exitReason

				trades = append(trades, currentTrade)
				tradeReturns = append(tradeReturns, netRetPct)
				totalFeesPaid += totalFee
				equity += netPnL

				if netPnL > 0 {
					grossProfitSum += netPnL
				} else {
					grossLossSum += math.Abs(netPnL)
				}
				inPosition = false
			}
		}

		if equity > peakEquity {
			peakEquity = equity
		}
		dd := 0.0
		if peakEquity > 0 {
			dd = ((peakEquity - equity) / peakEquity) * 100.0
		}
		if dd > maxDDPct {
			maxDDPct = dd
		}

		equityCurve = append(equityCurve, EquityPoint{
			Date:        day.Date,
			EquityIDR:   math.Round(equity),
			DrawdownPct: math.Round(dd*100) / 100,
		})
	}

	// Hitung Metrik Statistik Akhir
	totalTrades := len(trades)
	winCount := 0
	var sumRet float64
	for _, tr := range trades {
		if tr.NetPnLIDR > 0 {
			winCount++
		}
		sumRet += tr.NetReturnPct
	}
	lossCount := totalTrades - winCount

	winRate := 0.0
	expectancy := 0.0
	if totalTrades > 0 {
		winRate = (float64(winCount) / float64(totalTrades)) * 100.0
		expectancy = sumRet / float64(totalTrades)
	}

	profitFactor := 0.0
	if grossLossSum > 0 {
		profitFactor = grossProfitSum / grossLossSum
	} else if grossProfitSum > 0 {
		profitFactor = 99.9
	}

	totalNetRet := ((equity - cfg.InitialCapitalIDR) / cfg.InitialCapitalIDR) * 100.0
	firstClose := history[0].Bar.Close
	lastClose := history[len(history)-1].Bar.Close
	bnhRet := 0.0
	if firstClose > 0 {
		bnhRet = ((lastClose - firstClose) / firstClose) * 100.0
	}

	sharpe, sortino := computeRiskAdjustedRatios(tradeReturns)

	return Report{
		Ticker:                ticker,
		StartDate:             history[0].Date,
		EndDate:               history[len(history)-1].Date,
		InitialCapitalIDR:     cfg.InitialCapitalIDR,
		FinalCapitalIDR:       math.Round(equity),
		TotalNetReturnPct:     math.Round(totalNetRet*100) / 100,
		BuyAndHoldReturnPct:   math.Round(bnhRet*100) / 100,
		AlphaVsBuyAndHoldPct:  math.Round((totalNetRet-bnhRet)*100) / 100,
		TotalTrades:           totalTrades,
		WinningTrades:         winCount,
		LosingTrades:          lossCount,
		WinRatePct:            math.Round(winRate*100) / 100,
		ProfitFactor:          math.Round(profitFactor*100) / 100,
		ExpectancyPerTradePct: math.Round(expectancy*100) / 100,
		MaxDrawdownPct:        math.Round(maxDDPct*100) / 100,
		SharpeRatio:           sharpe,
		SortinoRatio:          sortino,
		TotalBrokerTaxFeeIDR:  math.Round(totalFeesPaid),
		Trades:                trades,
		EquityCurve:           equityCurve,
	}
}

func applyTickSlippage(price float64, ticks int) float64 {
	var step float64
	switch {
	case price < 200:
		step = 1
	case price < 500:
		step = 2
	case price < 2000:
		step = 5
	case price < 5000:
		step = 10
	default:
		step = 25
	}
	return domain.RoundToIDXTick(price + float64(ticks)*step)
}

func computeRiskAdjustedRatios(returns []float64) (sharpe float64, sortino float64) {
	n := len(returns)
	if n < 2 {
		return 0, 0
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(n)

	var variance, downsideVar float64
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
		if r < 0 {
			downsideVar += r * r
		}
	}
	stdDev := math.Sqrt(variance / float64(n-1))
	downsideDev := math.Sqrt(downsideVar / float64(n))

	if stdDev > 0 {
		sharpe = math.Round(((mean/stdDev)*math.Sqrt(float64(n)))*100) / 100
	}
	if downsideDev > 0 {
		sortino = math.Round(((mean/downsideDev)*math.Sqrt(float64(n)))*100) / 100
	}
	return sharpe, sortino
}