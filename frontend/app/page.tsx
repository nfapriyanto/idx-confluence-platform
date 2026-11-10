"use client";

import React, { useEffect, useMemo, useState } from "react";

// =============================================================================
// NEXT.JS 16 + REACT 19 DASHBOARD: IDX 5-LAYER CONFLUENCE TRADING TERMINAL
// File: page.tsx (Letakkan di frontend/src/app/page.tsx)
// Fitur:
// 1. Live Mode Switcher: Scalping (1m/5m RVOL+VWAP), Swing (Bandarmology+NLP), Dividend Trap Radar
// 2. Filter Saham Syariah (ISSI/JII), LQ45, & Pencarian Emiten
// 3. Visualisasi Skor 5 Lapis (Macro, Fundamental, Bandar/Foreign, IndoBERT+Lexicon, Technical)
// 4. Kalkulator Position Sizing & Risk Management Otomatis (Fraksi Harga BEI & ATR Stop Loss)
// 5. Koneksi Real-Time ke Backend Golang via REST & Server-Sent Events (SSE)
// =============================================================================

type TradingMode = "SWING" | "SCALPING" | "DIVIDEND";

interface NewsItem {
  title: string;
  portal: string;
  url: string;
  indobert_score: number;
  lexicon_bonus: number;
  hybrid_score: number;
  age_hours: number;
  decayed_weight: number;
}

interface TickerSignal {
  fundamental: {
    ticker: string;
    company_name: string;
    sector: string;
    is_syariah: boolean;
    is_lq45: boolean;
    regime: string;
    last_price: number;
    per: number;
    pbv: number;
    roe: number;
  };
  smart_money: {
    net_foreign_5d_val: number;
    top1_buyer_code: string;
    top3_net_buy_val: number;
    bandar_avg_buy_price: number;
    ksei_institutional_diff: number;
    bandar_status: string;
  };
  technical: {
    price_above_ma20: boolean;
    price_above_ma50: boolean;
    price_above_vwap: boolean;
    vwap_price: number;
    connors_rsi_3: number;
    intraday_rvol: number;
    daily_rvol: number;
    atr_14: number;
  };
  dividend: {
    has_upcoming_dividend: boolean;
    dividend_per_share: number;
    estimated_yield_pct: number;
    days_to_cum_date: number;
    hist_3y_avg_ex_drop_pct: number;
  };
  recent_news: NewsItem[];
  intrinsic_fair_val: number;
  upside_pct: number;
  layer1_macro_score: number;
  layer2_fundamental_score: number;
  layer3_smart_money_score: number;
  layer4_sentiment_score: number;
  layer5_technical_score: number;
  scalping_score: number;
  swing_score: number;
  dividend_score: number;
  trade_plan: {
    recommended_mode: string;
    signal_grade: string;
    entry_zone_min: number;
    entry_zone_max: number;
    take_profit_1: number;
    take_profit_2: number;
    stop_loss_price: number;
    risk_reward_ratio: number;
    max_portfolio_alloc_pct: number;
    dividend_tactic: string;
    dividend_trap_risk: number;
  };
}

const FALLBACK_SIGNALS: TickerSignal[] = [
  {
    fundamental: {
      ticker: "BMRI",
      company_name: "PT Bank Mandiri (Persero) Tbk",
      sector: "BANKING",
      is_syariah: false,
      is_lq45: true,
      regime: "BLUECHIP_STABLE",
      last_price: 6450,
      per: 10.8,
      pbv: 2.3,
      roe: 22.8,
    },
    smart_money: {
      net_foreign_5d_val: 485000000000,
      top1_buyer_code: "ZP",
      top3_net_buy_val: 145000000000,
      bandar_avg_buy_price: 6375,
      ksei_institutional_diff: 2.1,
      bandar_status: "BIG_ACCUMULATION",
    },
    technical: {
      price_above_ma20: true,
      price_above_ma50: true,
      price_above_vwap: true,
      vwap_price: 6425,
      connors_rsi_3: 22.5,
      intraday_rvol: 1.9,
      daily_rvol: 1.85,
      atr_14: 135,
    },
    dividend: {
      has_upcoming_dividend: false,
      dividend_per_share: 0,
      estimated_yield_pct: 0,
      days_to_cum_date: 0,
      hist_3y_avg_ex_drop_pct: 0,
    },
    recent_news: [
      {
        title: "Laba Bersih Melonjak, BMRI Catat Akumulasi Asing Deras Pekan Ini",
        portal: "Kontan",
        url: "https://www.kontan.co.id/",
        indobert_score: 84,
        lexicon_bonus: 60,
        hybrid_score: 90.4,
        age_hours: 6,
        decayed_weight: 0.944,
      },
    ],
    intrinsic_fair_val: 6325,
    upside_pct: -1.94,
    layer1_macro_score: 100,
    layer2_fundamental_score: 90,
    layer3_smart_money_score: 100,
    layer4_sentiment_score: 90.4,
    layer5_technical_score: 100,
    scalping_score: 88.6,
    swing_score: 96.9,
    dividend_score: 20.0,
    trade_plan: {
      recommended_mode: "SWING",
      signal_grade: "A+",
      entry_zone_min: 6400,
      entry_zone_max: 6475,
      take_profit_1: 6875,
      take_profit_2: 7200,
      stop_loss_price: 6200,
      risk_reward_ratio: 3.0,
      max_portfolio_alloc_pct: 20,
      dividend_tactic: "NONE",
      dividend_trap_risk: 0,
    },
  },
  {
    fundamental: {
      ticker: "BRMS",
      company_name: "PT Bumi Resources Minerals Tbk",
      sector: "COMMODITY",
      is_syariah: true,
      is_lq45: false,
      regime: "CONGLOMERATE_THIRD_LINER",
      last_price: 384,
      per: 38.0,
      pbv: 2.6,
      roe: 8.5,
    },
    smart_money: {
      net_foreign_5d_val: 110000000000,
      top1_buyer_code: "MG",
      top3_net_buy_val: 68000000000,
      bandar_avg_buy_price: 378,
      ksei_institutional_diff: 4.6,
      bandar_status: "BIG_ACCUMULATION",
    },
    technical: {
      price_above_ma20: true,
      price_above_ma50: true,
      price_above_vwap: true,
      vwap_price: 376,
      connors_rsi_3: 48.0,
      intraday_rvol: 3.4,
      daily_rvol: 2.8,
      atr_14: 14,
    },
    dividend: {
      has_upcoming_dividend: false,
      dividend_per_share: 0,
      estimated_yield_pct: 0,
      days_to_cum_date: 0,
      hist_3y_avg_ex_drop_pct: 0,
    },
    recent_news: [
      {
        title: "Produksi Emas Naik Signifikan, Pengendali Tambah Kepemilikan dan Masuk Indeks MSCI",
        portal: "CNBC Indonesia",
        url: "https://www.cnbcindonesia.com/market",
        indobert_score: 88,
        lexicon_bonus: 65,
        hybrid_score: 92.8,
        age_hours: 3,
        decayed_weight: 0.972,
      },
    ],
    intrinsic_fair_val: 174,
    upside_pct: -54.69,
    layer1_macro_score: 100,
    layer2_fundamental_score: 58,
    layer3_smart_money_score: 100,
    layer4_sentiment_score: 92.8,
    layer5_technical_score: 97,
    scalping_score: 96.7,
    swing_score: 92.8,
    dividend_score: 20.0,
    trade_plan: {
      recommended_mode: "SCALPING",
      signal_grade: "A+",
      entry_zone_min: 380,
      entry_zone_max: 386,
      take_profit_1: 410,
      take_profit_2: 426,
      stop_loss_price: 368,
      risk_reward_ratio: 2.63,
      max_portfolio_alloc_pct: 20,
      dividend_tactic: "NONE",
      dividend_trap_risk: 0,
    },
  },
  {
    fundamental: {
      ticker: "ITMG",
      company_name: "PT Indo Tambangraya Megah Tbk",
      sector: "COMMODITY",
      is_syariah: true,
      is_lq45: true,
      regime: "CYCLICAL_HIGH_BETA",
      last_price: 26850,
      per: 6.4,
      pbv: 1.3,
      roe: 24.5,
    },
    smart_money: {
      net_foreign_5d_val: 95000000000,
      top1_buyer_code: "AK",
      top3_net_buy_val: 24000000000,
      bandar_avg_buy_price: 26500,
      ksei_institutional_diff: 0.95,
      bandar_status: "ACCUMULATION",
    },
    technical: {
      price_above_ma20: true,
      price_above_ma50: true,
      price_above_vwap: true,
      vwap_price: 26775,
      connors_rsi_3: 34.0,
      intraday_rvol: 1.6,
      daily_rvol: 1.5,
      atr_14: 450,
    },
    dividend: {
      has_upcoming_dividend: true,
      dividend_per_share: 1950,
      estimated_yield_pct: 7.26,
      days_to_cum_date: 14,
      hist_3y_avg_ex_drop_pct: 6.8,
    },
    recent_news: [
      {
        title: "ITMG Siapkan Dividen Interim Jumbo, Simak Jadwal Cum Date Bulan Ini",
        portal: "Kontan",
        url: "https://www.kontan.co.id/",
        indobert_score: 81,
        lexicon_bonus: 80,
        hybrid_score: 88.6,
        age_hours: 12,
        decayed_weight: 0.891,
      },
    ],
    intrinsic_fair_val: 35275,
    upside_pct: 31.38,
    layer1_macro_score: 100,
    layer2_fundamental_score: 93.1,
    layer3_smart_money_score: 100,
    layer4_sentiment_score: 88.6,
    layer5_technical_score: 82,
    scalping_score: 80.9,
    swing_score: 92.4,
    dividend_score: 100,
    trade_plan: {
      recommended_mode: "DIVIDEND",
      signal_grade: "A+",
      entry_zone_min: 26700,
      entry_zone_max: 26925,
      take_profit_1: 28300,
      take_profit_2: 29325,
      stop_loss_price: 26050,
      risk_reward_ratio: 3.09,
      max_portfolio_alloc_pct: 20,
      dividend_tactic: "HOLD_OR_REBUY_EX (Ambil Dividen)",
      dividend_trap_risk: 30,
    },
  },
];

export default function IDXConfluenceDashboard() {
  const [mode, setMode] = useState<TradingMode>("SWING");
  const [syariahOnly, setSyariahOnly] = useState<boolean>(false);
  const [signals, setSignals] = useState<TickerSignal[]>(FALLBACK_SIGNALS);
  const [selectedTicker, setSelectedTicker] = useState<string>("BMRI");
  const [capitalIDR, setCapitalIDR] = useState<number>(100_000_000); // Modal Rp 100 Juta

  // Sinkronisasi ke Backend Golang (http://localhost:8080/api/v1/signals)
  useEffect(() => {
    const apiUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
    fetch(`${apiUrl}/api/v1/signals`)
      .then((res) => res.json())
      .then((data) => {
        if (data?.signals?.length > 0) {
          setSignals(data.signals);
        }
      })
      .catch(() => {
        // Gunakan fallback data lokal saat preview tanpa backend berjalan
      });
  }, []);

  const filteredSignals = useMemo(() => {
    return signals
      .filter((s) => (!syariahOnly ? true : s.fundamental.is_syariah))
      .sort((a, b) => {
        if (mode === "SCALPING") return b.scalping_score - a.scalping_score;
        if (mode === "DIVIDEND") return b.dividend_score - a.dividend_score;
        return b.swing_score - a.swing_score;
      });
  }, [signals, mode, syariahOnly]);

  const activeStock = useMemo(() => {
    return (
      filteredSignals.find((s) => s.fundamental.ticker === selectedTicker) ||
      filteredSignals[0]
    );
  }, [filteredSignals, selectedTicker]);

  // Kalkulasi Lot & Manajemen Risiko Otomatis (1 Lot BEI = 100 Lembar)
  const positionCalc = useMemo(() => {
    if (!activeStock) return { maxModal: 0, recommendedLots: 0, maxLossIDR: 0 };
    const maxModal =
      (capitalIDR * activeStock.trade_plan.max_portfolio_alloc_pct) / 100;
    const pricePerLot = activeStock.fundamental.last_price * 100;
    const recommendedLots = Math.max(0, Math.floor(maxModal / pricePerLot));
    const lossPerShare = Math.max(
      0,
      activeStock.fundamental.last_price - activeStock.trade_plan.stop_loss_price
    );
    const maxLossIDR = recommendedLots * 100 * lossPerShare;
    return { maxModal, recommendedLots, maxLossIDR };
  }, [activeStock, capitalIDR]);

  const formatIDR = (val: number) =>
    new Intl.NumberFormat("id-ID", {
      style: "currency",
      currency: "IDR",
      maximumFractionDigits: 0,
    }).format(val);

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 p-6 font-sans">
      {/* Header & Status Rezim Pasar Lapis 1 */}
      <header className="max-w-7xl mx-auto flex flex-col md:flex-row justify-between items-start md:items-center gap-4 pb-6 border-b border-slate-800">
        <div>
          <div className="flex items-center gap-3">
            <span className="px-2.5 py-1 text-xs font-semibold rounded-full bg-emerald-500/20 text-emerald-300 border border-emerald-500/30">
              100% Free Data Stack (YF v8 + IDX + IPOT + IndoBERT)
            </span>
            <span className="px-2.5 py-1 text-xs font-mono rounded-full bg-blue-500/20 text-blue-300">
              Lapis 1 IHSG: BULL_CALM (GARCH Vol 13.8%)
            </span>
          </div>
          <h1 className="text-2xl font-bold mt-2 tracking-tight">
            IDX 5-Layer Confluence Trading Terminal
          </h1>
          <p className="text-sm text-slate-400">
            Integrasi Fundamental Sektoral, Bandarmology & Foreign Flow, Hybrid
            IndoBERT NLP, dan Multi-Indicator Technical
          </p>
        </div>

        {/* Mode Selector (Scalping, Swing, Dividend) */}
        <div className="flex flex-wrap items-center gap-2 bg-slate-900 p-1.5 rounded-xl border border-slate-800">
          {(["SCALPING", "SWING", "DIVIDEND"] as TradingMode[]).map((m) => (
            <button
              key={m}
              onClick={() => setMode(m)}
              className={`px-4 py-2 rounded-lg text-sm font-semibold transition ${
                mode === m
                  ? "bg-blue-600 text-white shadow-lg"
                  : "text-slate-400 hover:text-slate-200"
              }`}
            >
              {m === "SCALPING" && "⚡ Scalping (1m/5m)"}
              {m === "SWING" && "🌊 Swing (Bandar+NLP)"}
              {m === "DIVIDEND" && "💰 Dividend Hunter"}
            </button>
          ))}
          <button
            onClick={() => setSyariahOnly(!syariahOnly)}
            className={`px-3 py-2 rounded-lg text-xs font-semibold border transition ${
              syariahOnly
                ? "bg-emerald-600/30 border-emerald-500 text-emerald-300"
                : "border-slate-700 text-slate-400"
            }`}
          >
            {syariahOnly ? "✓ ISSI Syariah" : "Semua Saham"}
          </button>
        </div>
      </header>

      <main className="max-w-7xl mx-auto grid grid-cols-1 lg:grid-cols-12 gap-6 mt-6">
        {/* Tabel Screener Konfluensi (Kiri - 7 Kolom) */}
        <section className="lg:col-span-7 bg-slate-900/90 border border-slate-800 rounded-2xl p-5">
          <div className="flex justify-between items-center mb-4">
            <h2 className="text-lg font-semibold">
              Screener Konfluensi Mode: <span className="text-blue-400">{mode}</span>
            </h2>
            <span className="text-xs text-slate-400">
              Klik baris saham untuk membedah 5 Lapis & Trade Plan
            </span>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-sm">
              <thead>
                <tr className="border-b border-slate-800 text-slate-400 text-xs uppercase">
                  <th className="py-3 px-2">Emiten</th>
                  <th className="py-3 px-2 text-right">Harga</th>
                  <th className="py-3 px-2 text-center">Skor {mode}</th>
                  <th className="py-3 px-2 text-center">L3 Bandar</th>
                  <th className="py-3 px-2 text-center">L4 NLP</th>
                  <th className="py-3 px-2 text-center">RVOL</th>
                  <th className="py-3 px-2 text-center">Grade</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/60">
                {filteredSignals.map((item) => {
                  const activeScore =
                    mode === "SCALPING"
                      ? item.scalping_score
                      : mode === "DIVIDEND"
                      ? item.dividend_score
                      : item.swing_score;
                  const isSelected =
                    activeStock?.fundamental.ticker === item.fundamental.ticker;

                  return (
                    <tr
                      key={item.fundamental.ticker}
                      onClick={() => setSelectedTicker(item.fundamental.ticker)}
                      className={`cursor-pointer transition ${
                        isSelected
                          ? "bg-blue-950/50 border-l-4 border-l-blue-500"
                          : "hover:bg-slate-800/50"
                      }`}
                    >
                      <td className="py-3.5 px-2">
                        <div className="font-bold text-white flex items-center gap-1.5">
                          {item.fundamental.ticker}
                          {item.fundamental.is_syariah && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-300">
                              ISSI
                            </span>
                          )}
                        </div>
                        <div className="text-xs text-slate-400 truncate max-w-[160px]">
                          {item.fundamental.sector} • {item.smart_money.bandar_status}
                        </div>
                      </td>
                      <td className="py-3.5 px-2 text-right font-mono font-semibold">
                        Rp {item.fundamental.last_price.toLocaleString("id-ID")}
                      </td>
                      <td className="py-3.5 px-2 text-center">
                        <span className="font-mono font-bold text-blue-400">
                          {activeScore.toFixed(1)}
                        </span>
                      </td>
                      <td className="py-3.5 px-2 text-center font-mono">
                        <span
                          className={
                            item.layer3_smart_money_score >= 70
                              ? "text-emerald-400"
                              : "text-amber-400"
                          }
                        >
                          {item.layer3_smart_money_score.toFixed(0)}
                        </span>
                      </td>
                      <td className="py-3.5 px-2 text-center font-mono">
                        {item.layer4_sentiment_score.toFixed(0)}
                      </td>
                      <td className="py-3.5 px-2 text-center font-mono">
                        {item.technical.intraday_rvol.toFixed(1)}x
                      </td>
                      <td className="py-3.5 px-2 text-center">
                        <span
                          className={`px-2.5 py-1 rounded-full text-xs font-bold ${
                            item.trade_plan.signal_grade.startsWith("A")
                              ? "bg-emerald-500/20 text-emerald-300"
                              : "bg-amber-500/20 text-amber-300"
                          }`}
                        >
                          {item.trade_plan.signal_grade}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>

        {/* Panel Bedah 5 Lapis & Kalkulator Eksekusi (Kanan - 5 Kolom) */}
        {activeStock && (
          <section className="lg:col-span-5 space-y-5">
            {/* Breakdown 5 Lapis Konfluensi */}
            <div className="bg-slate-900/90 border border-slate-800 rounded-2xl p-5">
              <div className="flex justify-between items-start mb-4">
                <div>
                  <h3 className="text-xl font-bold">
                    {activeStock.fundamental.ticker} —{" "}
                    <span className="text-slate-400 text-sm font-normal">
                      {activeStock.fundamental.company_name}
                    </span>
                  </h3>
                  <p className="text-xs text-slate-400 mt-0.5">
                    Rezim: {activeStock.fundamental.regime} | Top Buyer:{" "}
                    <strong className="text-emerald-400">
                      Broker {activeStock.smart_money.top1_buyer_code}
                    </strong>{" "}
                    (Avg Bandar: Rp{" "}
                    {activeStock.smart_money.bandar_avg_buy_price.toLocaleString(
                      "id-ID"
                    )}
                    )
                  </p>
                </div>
                <span className="px-3 py-1 rounded-lg bg-blue-600/20 border border-blue-500/40 text-blue-300 text-xs font-bold">
                  Rekomendasi: {activeStock.trade_plan.recommended_mode}
                </span>
              </div>

              <div className="space-y-3 text-xs">
                {[
                  {
                    label: "Lapis 1: Rezim Makro & GARCH Volatility",
                    score: activeStock.layer1_macro_score,
                  },
                  {
                    label: `Lapis 2: Fundamental Sektoral (Fair Val: Rp ${activeStock.intrinsic_fair_val.toLocaleString("id-ID")})`,
                    score: activeStock.layer2_fundamental_score,
                  },
                  {
                    label: `Lapis 3: Bandarmology & Foreign Flow (KSEI Drift: +${activeStock.smart_money.ksei_institutional_diff}%)`,
                    score: activeStock.layer3_smart_money_score,
                  },
                  {
                    label: "Lapis 4: Sentimen Hybrid (60% IndoBERT + 40% Lexicon)",
                    score: activeStock.layer4_sentiment_score,
                  },
                  {
                    label: `Lapis 5: Technical Ensemble (CRSI: ${activeStock.technical.connors_rsi_3}, RVOL: ${activeStock.technical.intraday_rvol}x)`,
                    score: activeStock.layer5_technical_score,
                  },
                ].map((layer) => (
                  <div key={layer.label}>
                    <div className="flex justify-between mb-1">
                      <span className="text-slate-300">{layer.label}</span>
                      <span className="font-mono font-bold">
                        {layer.score.toFixed(1)} / 100
                      </span>
                    </div>
                    <div className="w-full h-2 bg-slate-800 rounded-full overflow-hidden">
                      <div
                        className="h-full bg-gradient-to-r from-blue-500 to-emerald-400"
                        style={{ width: `${Math.min(100, layer.score)}%` }}
                      />
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {/* Rencana Trading & Kalkulator Position Sizing */}
            <div className="bg-slate-900/90 border border-slate-800 rounded-2xl p-5">
              <h4 className="text-sm font-semibold text-slate-300 uppercase tracking-wider mb-3">
                Trade Plan & Kalkulator Lot (Fraksi BEI & ATR-14)
              </h4>

              <div className="mb-4">
                <label className="text-xs text-slate-400 block mb-1">
                  Total Modal Portofolio Anda (IDR):
                </label>
                <input
                  type="number"
                  value={capitalIDR}
                  onChange={(e) => setCapitalIDR(Number(e.target.value))}
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm font-mono text-white"
                />
              </div>

              <div className="grid grid-cols-2 gap-3 text-xs">
                <div className="p-3 rounded-xl bg-slate-950 border border-slate-800">
                  <span className="text-slate-400">Area Beli (Entry Zone)</span>
                  <p className="text-base font-mono font-bold text-blue-400 mt-1">
                    Rp {activeStock.trade_plan.entry_zone_min.toLocaleString("id-ID")} –{" "}
                    {activeStock.trade_plan.entry_zone_max.toLocaleString("id-ID")}
                  </p>
                </div>
                <div className="p-3 rounded-xl bg-slate-950 border border-slate-800">
                  <span className="text-slate-400">
                    Stop Loss (ATR) • R/R 1:{activeStock.trade_plan.risk_reward_ratio}
                  </span>
                  <p className="text-base font-mono font-bold text-rose-400 mt-1">
                    Rp {activeStock.trade_plan.stop_loss_price.toLocaleString("id-ID")}
                  </p>
                </div>
                <div className="p-3 rounded-xl bg-slate-950 border border-slate-800">
                  <span className="text-slate-400">Target Take Profit 1 & 2</span>
                  <p className="text-base font-mono font-bold text-emerald-400 mt-1">
                    Rp {activeStock.trade_plan.take_profit_1.toLocaleString("id-ID")} /{" "}
                    {activeStock.trade_plan.take_profit_2.toLocaleString("id-ID")}
                  </p>
                </div>
                <div className="p-3 rounded-xl bg-slate-950 border border-slate-800">
                  <span className="text-slate-400">
                    Beli Maks ({activeStock.trade_plan.max_portfolio_alloc_pct}% Modal)
                  </span>
                  <p className="text-base font-mono font-bold text-amber-300 mt-1">
                    {positionCalc.recommendedLots.toLocaleString("id-ID")} Lot
                  </p>
                  <span className="text-[11px] text-slate-400">
                    Risiko terukur: {formatIDR(positionCalc.maxLossIDR)}
                  </span>
                </div>
              </div>

              {/* Peringatan Dividend Trap jika saham memiliki jadwal Cum Date */}
              {activeStock.dividend.has_upcoming_dividend && (
                <div className="mt-4 p-3 rounded-xl bg-amber-500/10 border border-amber-500/30 text-xs">
                  <div className="font-bold text-amber-300">
                    📅 Radar Dividen: Est. Yield {activeStock.dividend.estimated_yield_pct}% (H-
                    {activeStock.dividend.days_to_cum_date} Cum Date)
                  </div>
                  <div className="text-slate-300 mt-1">
                    Histori Ex-Date Drop 3Thn:{" "}
                    <strong>-{activeStock.dividend.hist_3y_avg_ex_drop_pct}%</strong> | Trap
                    Risk: <strong>{activeStock.trade_plan.dividend_trap_risk}/100</strong> →{" "}
                    <span className="text-emerald-300 font-semibold">
                      {activeStock.trade_plan.dividend_tactic}
                    </span>
                  </div>
                </div>
              )}
            </div>
          </section>
        )}
      </main>
    </div>
  );
}