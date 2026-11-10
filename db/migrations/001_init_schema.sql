-- =============================================================================
-- IDX 5-LAYER CONFLUENCE TRADING SYSTEM - POSTGRESQL 17 + TIMESCALEDB SCHEMA
-- File: 001_init_schema.sql
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS timescaledb;

-- 1. MASTER EMITEN & KLASIFIKASI REZIM VOLATILITAS (LAPIS 2)
CREATE TABLE IF NOT EXISTS tickers (
    ticker VARCHAR(10) PRIMARY KEY,
    company_name VARCHAR(160) NOT NULL,
    sector VARCHAR(64) NOT NULL, -- BANKING, COMMODITY, CONSUMER, TECH, INFRA, PROPERTY
    board VARCHAR(32) NOT NULL DEFAULT 'UTAMA', -- UTAMA, PENGEMBANGAN, PEMANTAUAN_KHUSUS
    is_syariah BOOLEAN NOT NULL DEFAULT FALSE,
    is_lq45 BOOLEAN NOT NULL DEFAULT FALSE,
    regime_type VARCHAR(32) NOT NULL DEFAULT 'BLUECHIP_STABLE', -- BLUECHIP_STABLE, CYCLICAL_HIGH_BETA, CONGLOMERATE_THIRD_LINER
    per_ratio NUMERIC(10, 2) DEFAULT 0,
    pbv_ratio NUMERIC(10, 2) DEFAULT 0,
    roe_pct NUMERIC(10, 2) DEFAULT 0,
    der_ratio NUMERIC(10, 2) DEFAULT 0,
    eps NUMERIC(14, 2) DEFAULT 0,
    bvps NUMERIC(14, 2) DEFAULT 0,
    npl_proxy_pct NUMERIC(6, 2) DEFAULT 0,
    free_float_pct NUMERIC(6, 2) DEFAULT 15.0,
    ksei_institutional_drift_pct NUMERIC(8, 2) DEFAULT 0, -- Perubahan kepemilikan >1% KSEI bulanan
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. HYPERTABLE OHLCV INTRADAY (1m / 5m UNTUK MODE SCALPING - LAPIS 5)
CREATE TABLE IF NOT EXISTS ohlcv_intraday (
    bucket_time TIMESTAMPTZ NOT NULL,
    ticker VARCHAR(10) NOT NULL REFERENCES tickers(ticker) ON DELETE CASCADE,
    open_price NUMERIC(12, 2) NOT NULL,
    high_price NUMERIC(12, 2) NOT NULL,
    low_price NUMERIC(12, 2) NOT NULL,
    close_price NUMERIC(12, 2) NOT NULL,
    volume BIGINT NOT NULL,
    vwap NUMERIC(12, 2) NOT NULL DEFAULT 0,
    rvol_ratio NUMERIC(8, 2) NOT NULL DEFAULT 1.0, -- Relative Volume thd rata-rata 20 bar
    PRIMARY KEY (bucket_time, ticker)
);

SELECT create_hypertable('ohlcv_intraday', 'bucket_time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_ohlcv_intraday_ticker ON ohlcv_intraday (ticker, bucket_time DESC);

-- 3. HYPERTABLE OHLCV HARIAN & ARUS DANA ASING (UNTUK SWING & DIVIDEND - LAPIS 1, 3, 5)
CREATE TABLE IF NOT EXISTS ohlcv_daily (
    trade_date TIMESTAMPTZ NOT NULL,
    ticker VARCHAR(10) NOT NULL REFERENCES tickers(ticker) ON DELETE CASCADE,
    open_price NUMERIC(12, 2) NOT NULL,
    high_price NUMERIC(12, 2) NOT NULL,
    low_price NUMERIC(12, 2) NOT NULL,
    close_price NUMERIC(12, 2) NOT NULL,
    volume BIGINT NOT NULL,
    value_idr NUMERIC(20, 2) NOT NULL DEFAULT 0,
    frequency INT NOT NULL DEFAULT 0,
    foreign_buy_val NUMERIC(20, 2) NOT NULL DEFAULT 0,
    foreign_sell_val NUMERIC(20, 2) NOT NULL DEFAULT 0,
    net_foreign_val NUMERIC(20, 2) GENERATED ALWAYS AS (foreign_buy_val - foreign_sell_val) STORED,
    atr_14 NUMERIC(12, 2) DEFAULT 0,
    crsi_3 NUMERIC(6, 2) DEFAULT 50.0, -- Connors RSI (3, 2, 100)
    PRIMARY KEY (trade_date, ticker)
);

SELECT create_hypertable('ohlcv_daily', 'trade_date', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_ohlcv_daily_ticker ON ohlcv_daily (ticker, trade_date DESC);

-- 4. RINGKASAN BROKER & BANDARMOLOGY HARIAN (POST-MARKET 16:30 WIB - LAPIS 3)
CREATE TABLE IF NOT EXISTS broker_summaries (
    trade_date DATE NOT NULL,
    ticker VARCHAR(10) NOT NULL REFERENCES tickers(ticker) ON DELETE CASCADE,
    top1_buyer_code VARCHAR(4),
    top3_net_buy_val NUMERIC(20, 2) NOT NULL DEFAULT 0,
    top5_net_buy_val NUMERIC(20, 2) NOT NULL DEFAULT 0,
    total_transaction_val NUMERIC(20, 2) NOT NULL DEFAULT 1,
    bandar_accumulation_ratio NUMERIC(8, 4) GENERATED ALWAYS AS (
        CASE WHEN total_transaction_val > 0 THEN (top3_net_buy_val / total_transaction_val) * 100 ELSE 0 END
    ) STORED,
    bandar_avg_buy_price NUMERIC(12, 2) NOT NULL DEFAULT 0,
    bandar_status VARCHAR(24) NOT NULL DEFAULT 'NEUTRAL', -- BIG_ACCUMULATION, ACCUMULATION, NEUTRAL, DISTRIBUTION
    PRIMARY KEY (trade_date, ticker)
);

-- 5. SENTIMEN BERITA HYBRID INDOBERT + LEXICON (LAPIS 4A)
CREATE TABLE IF NOT EXISTS news_sentiments (
    id BIGSERIAL PRIMARY KEY,
    published_at TIMESTAMPTZ NOT NULL,
    source_portal VARCHAR(48) NOT NULL, -- KONTAN, CNBC_INDONESIA, ANTARA, IDX_FILING
    title TEXT NOT NULL,
    article_url TEXT UNIQUE NOT NULL,
    primary_ticker VARCHAR(10) REFERENCES tickers(ticker) ON DELETE SET NULL,
    co_occurring_tickers TEXT[] DEFAULT '{}', -- Emiten satu sektor/grup yang terdampak spillover
    indobert_prob_pos NUMERIC(6, 4) NOT NULL DEFAULT 0.3333,
    indobert_prob_neg NUMERIC(6, 4) NOT NULL DEFAULT 0.3333,
    indobert_score NUMERIC(6, 2) NOT NULL DEFAULT 50.0, -- Skala 0-100
    lexicon_adjustment NUMERIC(6, 2) NOT NULL DEFAULT 0.0, -- Koreksi kamus pasar modal lokal
    hybrid_raw_score NUMERIC(6, 2) NOT NULL DEFAULT 50.0 -- (60% IndoBERT + 40% Lexicon)
);

CREATE INDEX IF NOT EXISTS idx_news_published ON news_sentiments (primary_ticker, published_at DESC);

-- 6. KALENDER DIVIDEN & TRAP RISK RADAR (LAPIS 4B)
CREATE TABLE IF NOT EXISTS dividend_events (
    id BIGSERIAL PRIMARY KEY,
    ticker VARCHAR(10) NOT NULL REFERENCES tickers(ticker) ON DELETE CASCADE,
    fiscal_year INT NOT NULL,
    dividend_per_share NUMERIC(12, 2) NOT NULL,
    estimated_yield_pct NUMERIC(6, 2) NOT NULL,
    cum_date DATE NOT NULL,
    ex_date DATE NOT NULL,
    payment_date DATE,
    payout_ratio_pct NUMERIC(6, 2) DEFAULT 50.0,
    hist_3y_avg_ex_drop_pct NUMERIC(6, 2) NOT NULL DEFAULT 4.5, -- Rata-rata penurunan harga saat Ex-Date
    dividend_trap_risk_score NUMERIC(6, 2) NOT NULL DEFAULT 50.0, -- 0 (Aman) s/d 100 (Sangat Berbahaya)
    recommended_tactic VARCHAR(32) NOT NULL DEFAULT 'PRE_CUM_EXIT', -- PRE_CUM_EXIT, HOLD_FOR_YIELD, AVOID_TRAP
    UNIQUE (ticker, cum_date)
);

-- 7. TABEL OUTPUT SKOR KONFLUENSI 5 LAPIS & REKOMENDASI EKSEKUSI (UNTUK DASHBOARD NEXT.JS)
CREATE TABLE IF NOT EXISTS confluence_signals (
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ticker VARCHAR(10) NOT NULL REFERENCES tickers(ticker) ON DELETE CASCADE,
    -- Skor masing-masing lapis (Skala 0 - 100)
    layer1_macro_regime_score NUMERIC(6, 2) NOT NULL,
    layer2_fundamental_score NUMERIC(6, 2) NOT NULL,
    layer3_smart_money_score NUMERIC(6, 2) NOT NULL,
    layer4_sentiment_catalyst_score NUMERIC(6, 2) NOT NULL,
    layer5_technical_trigger_score NUMERIC(6, 2) NOT NULL,
    -- Skor Gabungan 3 Mode
    scalping_total_score NUMERIC(6, 2) NOT NULL,
    swing_total_score NUMERIC(6, 2) NOT NULL,
    dividend_total_score NUMERIC(6, 2) NOT NULL,
    -- Rencana Perdagangan & Manajemen Risiko (Berbasis ATR & Aturan Fraksi/ARB BEI)
    last_price NUMERIC(12, 2) NOT NULL,
    bandar_avg_price NUMERIC(12, 2) NOT NULL,
    recommended_mode VARCHAR(20) NOT NULL, -- SCALPING, SWING, DIVIDEND, WAIT
    signal_grade VARCHAR(8) NOT NULL, -- A+, A, B, C, AVOID
    entry_zone_min NUMERIC(12, 2) NOT NULL,
    entry_zone_max NUMERIC(12, 2) NOT NULL,
    take_profit_1 NUMERIC(12, 2) NOT NULL,
    take_profit_2 NUMERIC(12, 2) NOT NULL,
    stop_loss_price NUMERIC(12, 2) NOT NULL,
    risk_reward_ratio NUMERIC(6, 2) NOT NULL,
    max_portfolio_alloc_pct NUMERIC(5, 2) NOT NULL,
    PRIMARY KEY (ticker)
);

-- =============================================================================
-- DATA SEED CONTOH (REPRESENTASI EMITEN BEI LINTAS REZIM)
-- =============================================================================
INSERT INTO tickers (ticker, company_name, sector, board, is_syariah, is_lq45, regime_type, per_ratio, pbv_ratio, roe_pct, der_ratio, eps, bvps, npl_proxy_pct, free_float_pct, ksei_institutional_drift_pct)
VALUES
    ('BBCA', 'PT Bank Central Asia Tbk', 'BANKING', 'UTAMA', FALSE, TRUE, 'BLUECHIP_STABLE', 21.4, 4.8, 21.5, 0.0, 445, 2050, 1.8, 44.9, 1.45),
    ('BMRI', 'PT Bank Mandiri (Persero) Tbk', 'BANKING', 'UTAMA', FALSE, TRUE, 'BLUECHIP_STABLE', 10.8, 2.3, 22.8, 0.0, 610, 2920, 1.2, 40.0, 2.10),
    ('ISAT', 'PT Indosat Tbk', 'INFRA', 'UTAMA', TRUE, TRUE, 'BLUECHIP_STABLE', 14.2, 2.5, 17.6, 1.4, 680, 3850, 0.0, 18.5, 1.80),
    ('ITMG', 'PT Indo Tambangraya Megah Tbk', 'COMMODITY', 'UTAMA', TRUE, TRUE, 'CYCLICAL_HIGH_BETA', 6.4, 1.3, 24.5, 0.2, 4150, 19800, 0.0, 34.8, 0.95),
    ('BRMS', 'PT Bumi Resources Minerals Tbk', 'COMMODITY', 'UTAMA', TRUE, FALSE, 'CONGLOMERATE_THIRD_LINER', 42.0, 2.8, 6.5, 0.3, 9, 135, 0.0, 48.2, 4.60),
    ('GOTO', 'PT GoTo Gojek Tokopedia Tbk', 'TECH', 'UTAMA', FALSE, TRUE, 'CYCLICAL_HIGH_BETA', -12.0, 1.6, -14.5, 0.1, -6, 42, 0.0, 72.0, -2.30)
ON CONFLICT (ticker) DO NOTHING;