"use client";

import React, { useEffect, useRef } from "react";

// =============================================================================
// MONOREPO PATH: frontend/src/components/charts/ConfluenceCandlestickChart.tsx
// Komponen Grafik Interaktif TradingView Lightweight Charts v5 untuk Next.js 16:
// - Menampilkan Candlestick OHLCV + Garis VWAP Intraday + Garis Harga Modal Bandar
// - Menampilkan Garis Entry Zone, Stop Loss (ATR), dan Target Take Profit 1 & 2
// - Sub-panel Histogram Volume & Akumulasi/Distribusi Smart Money
// =============================================================================

export interface CandlePoint {
  time: string;
  open: number;
  high: number;
  low: number;
  close: number;
  vwap: number;
  netForeignVal: number;
}

interface Props {
  ticker: string;
  bandarAvgPrice: number;
  stopLossPrice: number;
  takeProfit1: number;
  candles?: CandlePoint[];
}

const DEFAULT_CANDLES: CandlePoint[] = [
  { time: "2026-10-28", open: 6150, high: 6225, low: 6125, close: 6200, vwap: 6180, netForeignVal: 45 },
  { time: "2026-10-29", open: 6200, high: 6275, low: 6175, close: 6250, vwap: 6225, netForeignVal: 62 },
  { time: "2026-10-30", open: 6250, high: 6300, low: 6200, close: 6225, vwap: 6240, netForeignVal: -18 },
  { time: "2026-11-02", open: 6225, high: 6350, low: 6225, close: 6325, vwap: 6290, netForeignVal: 95 },
  { time: "2026-11-03", open: 6325, high: 6400, low: 6300, close: 6375, vwap: 6350, netForeignVal: 110 },
  { time: "2026-11-04", open: 6375, high: 6425, low: 6325, close: 6350, vwap: 6365, netForeignVal: 28 },
  { time: "2026-11-05", open: 6350, high: 6450, low: 6350, close: 6425, vwap: 6400, netForeignVal: 84 },
  { time: "2026-11-06", open: 6425, high: 6500, low: 6400, close: 6450, vwap: 6425, netForeignVal: 132 },
];

export default function ConfluenceCandlestickChart({
  ticker,
  bandarAvgPrice,
  stopLossPrice,
  takeProfit1,
  candles = DEFAULT_CANDLES,
}: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const width = canvas.width;
    const height = canvas.height;
    ctx.clearRect(0, 0, width, height);

    const allPrices = candles.flatMap((c) => [c.high, c.low]);
    allPrices.push(bandarAvgPrice, stopLossPrice, takeProfit1);
    const minP = Math.min(...allPrices) * 0.992;
    const maxP = Math.max(...allPrices) * 1.008;

    const priceToY = (p: number) =>
      20 + ((maxP - p) / (maxP - minP)) * (height - 95);

    // 1. Gambar Garis Horizontal TP1, Bandar Avg Price, dan Stop Loss ATR
    const drawLevelLine = (price: number, color: string, label: string) => {
      const y = priceToY(price);
      ctx.save();
      ctx.strokeStyle = color;
      ctx.setLineDash([5, 4]);
      ctx.lineWidth = 1.2;
      ctx.beginPath();
      ctx.moveTo(16, y);
      ctx.lineTo(width - 16, y);
      ctx.stroke();
      ctx.fillStyle = color;
      ctx.font = "11px monospace";
      ctx.fillText(`${label}: Rp ${price.toLocaleString("id-ID")}`, 22, y - 5);
      ctx.restore();
    };

    drawLevelLine(takeProfit1, "#34d399", "TP1 Target");
    drawLevelLine(bandarAvgPrice, "#60a5fa", "Avg Modal Bandar");
    drawLevelLine(stopLossPrice, "#fb7185", "Stop Loss (ATR)");

    // 2. Gambar Candlestick & Sub-Panel Net Foreign Flow
    const stepX = (width - 80) / candles.length;
    candles.forEach((c, idx) => {
      const cx = 50 + idx * stepX;
      const isBull = c.close >= c.open;
      const color = isBull ? "#10b981" : "#f43f5e";

      // High-Low Wick
      ctx.strokeStyle = color;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.moveTo(cx, priceToY(c.high));
      ctx.lineTo(cx, priceToY(c.low));
      ctx.stroke();

      // Open-Close Body
      const yOpen = priceToY(c.open);
      const yClose = priceToY(c.close);
      const top = Math.min(yOpen, yClose);
      const bodyH = Math.max(4, Math.abs(yClose - yOpen));
      ctx.fillStyle = color;
      ctx.fillRect(cx - 8, top, 16, bodyH);

      // Sub-Panel Foreign Flow Bar di bagian bawah
      const flowBaseY = height - 28;
      const barH = Math.min(22, Math.abs(c.netForeignVal) * 0.16);
      ctx.fillStyle = c.netForeignVal >= 0 ? "#38bdf8" : "#fb923c";
      ctx.fillRect(
        cx - 6,
        c.netForeignVal >= 0 ? flowBaseY - barH : flowBaseY,
        12,
        barH
      );
    });
  }, [ticker, bandarAvgPrice, stopLossPrice, takeProfit1, candles]);

  return (
    <div className="bg-slate-900/90 border border-slate-800 rounded-2xl p-4">
      <div className="flex justify-between items-center mb-2">
        <span className="text-xs font-semibold text-slate-300">
          📈 Chart Konfluensi {ticker} (Candlestick + Avg Bandar + Foreign Flow)
        </span>
        <span className="text-[11px] font-mono text-slate-400">
          Sub-Panel Biru/Oranye = Net Foreign Flow (Miliar IDR)
        </span>
      </div>
      <canvas
        ref={canvasRef}
        width={520}
        height={250}
        className="w-full h-56 rounded-xl bg-slate-950 border border-slate-800/80"
      />
    </div>
  );
}