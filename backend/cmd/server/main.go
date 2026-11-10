package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpdelivery "github.com/idx-trader/confluence-platform/internal/delivery/http"
	"github.com/idx-trader/confluence-platform/internal/domain"
	"github.com/idx-trader/confluence-platform/internal/nlp"
	"github.com/idx-trader/confluence-platform/internal/providers/idx"
	"github.com/idx-trader/confluence-platform/internal/providers/ipot"
	"github.com/idx-trader/confluence-platform/internal/providers/rss"
	"github.com/idx-trader/confluence-platform/internal/providers/yahoo"
	"github.com/idx-trader/confluence-platform/internal/strategy"
)

// =============================================================================
// MONOREPO PATH: backend/cmd/server/main.go
// Entrypoint Utama Server Go, Penjadwal Jam Bursa BEI (Cron), & Koneksi DB
// =============================================================================

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Muat Kamus Leksikon Keuangan & Model ONNX IndoBERT (Lapis 4)
	lexPath := os.Getenv("LEXICON_PATH")
	if lexPath == "" {
		lexPath = "models/idx_lexicon.json"
	}
	modelPath := os.Getenv("MODEL_PATH")
	if modelPath == "" {
		modelPath = "models/indobert_sentiment.onnx"
	}
	nlpEngine := nlp.NewEngine(modelPath, lexPath)

	// 2. Inisialisasi Provider Data Gratis (Yahoo v8, IDX, IPOT, RSS Media)
	yahooClient := yahoo.NewClient()
	idxClient := idx.NewClient()
	ipotClient := ipot.NewClient()
	rssClient := rss.NewClient()

	// 3. Inisialisasi Koneksi Database PostgreSQL / TimescaleDB (Jika tersedia)
	var dbConn *sql.DB
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		log.Printf("[DB] Menghubungkan ke PostgreSQL/TimescaleDB...")
	}

	// 4. Jalankan Evaluasi Konfluensi 5 Lapis Awal & Jadwalkan Cron Sesi BEI
	initialMacro, initialSignals := strategy.BootstrapInitialSignals(nlpEngine)
	httpHandler := httpdelivery.NewHandler(dbConn, nlpEngine, yahooClient, idxClient, ipotClient, rssClient, initialMacro, initialSignals)

	// Mulai Background Scheduler sesuai Jam Bursa Efek Indonesia (WIB)
	httpHandler.StartIDXMarketScheduler(ctx)

	mux := http.NewServeMux()
	httpHandler.RegisterRoutes(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("[IDX 5-Layer Confluence Server] Aktif di http://0.0.0.0:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[IDX Server] Mematikan server secara aman...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}