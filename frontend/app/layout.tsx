import type { Metadata } from "next";
import "./globals.css";

// =============================================================================
// MONOREPO PATH: frontend/src/app/layout.tsx
// Root Layout Wajib Next.js 16 App Router
// =============================================================================

export const metadata: Metadata = {
  title: "IDX 5-Layer Confluence Trading Terminal",
  description:
    "Sistem Pendukung Keputusan Trading Saham Indonesia (Scalping, Swing, & Dividend) Berbasis Fundamental Sektoral, Bandarmology, Foreign Flow, IndoBERT NLP, dan Multi-Indicator Technical.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="id" className="dark">
      <body className="bg-slate-950 text-slate-100 antialiased selection:bg-blue-500 selection:text-white">
        {children}
      </body>
    </html>
  );
}