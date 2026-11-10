package domain

import (
	"context"
	"math"
	"time"
)

// =============================================================================
// MONOREPO PATH: backend/internal/domain/models.go
// Kontrak Struct Murni, Interface Hexagonal Provider, & Fraksi Harga BEI
// =============================================================================

type RegimeType string

const (
	RegimeBlueChip     RegimeType = "BLUECHIP_STABLE"
	RegimeCyclical     RegimeType = "CYCLICAL_HIGH_BETA"
	RegimeConglomerate RegimeType = "CONGLOMERATE_THIRD_LINER"
)

type OHLCVBar struct {
	Timestamp time.Time `json:"timestamp"`
	Ticker    string    `json:"ticker"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    int64     `json:"volume"`
	VWAP      float64   `json:"vwap"`
	RVOL      float64   `json:"rvol"`
}

type DailyFlowRecord struct {
	Ticker         string  `json:"ticker"`
	ClosePrice     float64 `json:"close_price"`
	Volume         int64   `json:"volume"`
	ValueIDR       float64 `json:"value_idr"`
	Frequency      int     `json:"frequency"`
	ForeignBuyVal  float64 `json:"foreign_buy_val"`
	ForeignSellVal float64 `json:"foreign_sell_val"`
	NetForeignVal  float64 `json:"net_foreign_val"`
}

type MacroState struct {
	IHSGPrice         float64 `json:"ihsg_price"`
	IHSGMA20          float64 `json:"ihsg_ma20"`
	USDIDRRate        float64 `json:"usdidr_rate"`
	USDIDRChangePct   float64 `json:"usdidr_change_pct"`
	GarchVolatility   float64 `json:"garch_volatility_annualized"`
	MarketRegimeLabel string  `json:"market_regime_label"`
}

type FundamentalInput struct {
	Ticker      string     `json:"ticker"`
	CompanyName string     `json:"company_name"`
	Sector      string     `json:"sector"`
	IsSyariah   bool       `json:"is_syariah"`
	IsLQ45      bool       `json:"is_lq45"`
	Regime      RegimeType `json:"regime"`
	LastPrice   float64    `json:"last_price"`
	PER         float64    `json:"per"`
	PBV         float64    `json:"pbv"`
	ROE         float64    `json:"roe"`
	DER         float64    `json:"der"`
	EPS         float64    `json:"eps"`
	BVPS        float64    `json:"bvps"`
	NPLProxyPct float64    `json:"npl_proxy_pct"`
}

type SmartMoneyInput struct {
	NetForeign5DVal       float64 `json:"net_foreign_5d_val"`
	AvgDailyTransaction   float64 `json:"avg_daily_transaction"`
	Top1BuyerCode         string  `json:"top1_buyer_code"`
	Top3NetBuyVal         float64 `json:"top3_net_buy_val"`
	BandarAvgBuyPrice     float64 `json:"bandar_avg_buy_price"`
	KSEIInstitutionalDiff float64 `json:"ksei_institutional_diff"`
	BandarStatus          string  `json:"bandar_status"`
}

type NewsArticle struct {
	Title         string    `json:"title"`
	Portal        string    `json:"portal"`
	URL           string    `json:"url"`
	PublishedAt   time.Time `json:"published_at"`
	IndoBERTScore float64   `json:"indobert_score"`
	LexiconBonus  float64   `json:"lexicon_bonus"`
	HybridScore   float64   `json:"hybrid_score"`
	AgeHours      float64   `json:"age_hours"`
	DecayedWeight float64   `json:"decayed_weight"`
}

type DividendInput struct {
	HasUpcomingDividend bool    `json:"has_upcoming_dividend"`
	DividendPerShare    float64 `json:"dividend_per_share"`
	EstimatedYieldPct   float64 `json:"estimated_yield_pct"`
	DaysToCumDate       int     `json:"days_to_cum_date"`
	Hist3YAvgExDropPct  float64 `json:"hist_3y_avg_ex_drop_pct"`
	PayoutRatioPct      float64 `json:"payout_ratio_pct"`
}

type TechnicalInput struct {
	PriceAboveMA20 bool    `json:"price_above_ma20"`
	PriceAboveMA50 bool    `json:"price_above_ma50"`
	PriceAboveVWAP bool    `json:"price_above_vwap"`
	VWAPPrice      float64 `json:"vwap_price"`
	ConnorsRSI3    float64 `json:"connors_rsi_3"`
	RSI14          float64 `json:"rsi_14"`
	IntradayRVOL   float64 `json:"intraday_rvol"`
	DailyRVOL      float64 `json:"daily_rvol"`
	ATR14          float64 `json:"atr_14"`
	MACDHistogram  float64 `json:"macd_histogram"`
	OBVSlopePos    bool    `json:"obv_slope_positive"`
}

type TradePlan struct {
	RecommendedMode      string  `json:"recommended_mode"`
	SignalGrade          string  `json:"signal_grade"`
	EntryZoneMin         float64 `json:"entry_zone_min"`
	EntryZoneMax         float64 `json:"entry_zone_max"`
	TakeProfit1          float64 `json:"take_profit_1"`
	TakeProfit2          float64 `json:"take_profit_2"`
	StopLossPrice        float64 `json:"stop_loss_price"`
	RiskRewardRatio      float64 `json:"risk_reward_ratio"`
	MaxPortfolioAllocPct float64 `json:"max_portfolio_alloc_pct"`
	DividendTactic       string  `json:"dividend_tactic"`
	DividendTrapRisk     float64 `json:"dividend_trap_risk"`
}

type ConfluenceResult struct {
	CalculatedAt           time.Time        `json:"calculated_at"`
	Fundamental            FundamentalInput `json:"fundamental"`
	SmartMoney             SmartMoneyInput  `json:"smart_money"`
	Technical              TechnicalInput   `json:"technical"`
	Dividend               DividendInput    `json:"dividend"`
	RecentNews             []NewsArticle    `json:"recent_news"`
	IntrinsicFairVal       float64          `json:"intrinsic_fair_val"`
	UpsidePct              float64          `json:"upside_pct"`
	Layer1MacroScore       float64          `json:"layer1_macro_score"`
	Layer2FundamentalScore float64          `json:"layer2_fundamental_score"`
	Layer3SmartMoneyScore  float64          `json:"layer3_smart_money_score"`
	Layer4SentimentScore   float64          `json:"layer4_sentiment_score"`
	Layer5TechnicalScore   float64          `json:"layer5_technical_score"`
	ScalpingScore          float64          `json:"scalping_score"`
	SwingScore             float64          `json:"swing_score"`
	DividendScore          float64          `json:"dividend_score"`
	Plan                   TradePlan        `json:"trade_plan"`
}

type MarketQuoteProvider interface {
	FetchCandles(ctx context.Context, ticker string, interval string, rangePeriod string) ([]OHLCVBar, float64, error)
}

type SmartMoneyProvider interface {
	FetchBrokerSummary(ctx context.Context, ticker string, date string, totalDailyVal float64) (SmartMoneyInput, error)
}

type NewsFeedProvider interface {
	PollAllPortals(ctx context.Context, watchedTickers []string) ([]NewsArticle, error)
}

// RoundToIDXTick membulatkan harga sesuai aturan Fraksi Harga BEI
func RoundToIDXTick(price float64) float64 {
	if price <= 0 {
		return 50
	}
	var tick float64
	switch {
	case price < 200:
		tick = 1
	case price < 500:
		tick = 2
	case price < 2000:
		tick = 5
	case price < 5000:
		tick = 10
	default:
		tick = 25
	}
	return math.Round(price/tick) * tick
}

func ClampScore(val float64) float64 {
	if val < 0 {
		return 0
	}
	if val > 100 {
		return 100
	}
	return math.Round(val*100) / 100
}

func ClampDelta(val, minVal, maxVal float64) float64 {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}