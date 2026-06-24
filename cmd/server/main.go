// Package main initializes the application lifecycle, orchestrates dependency injection,
// verifies localized state parameters, and binds the high-performance HTTP service gateway
// with robust OS-level graceful shutdown handling.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Cfirth725/anime-sentinel/pkg/database"
	"github.com/Cfirth725/anime-sentinel/pkg/ingest"
	"github.com/Cfirth725/anime-sentinel/pkg/intelligence"
	"github.com/Cfirth725/anime-sentinel/pkg/metadata"
)

func main() {
	// 1. Initialize structured logging. Using slog key-value pairs ensures
	// the application telemetry is machine-readable and ready for log aggregators.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	slog.Info("Launching Anime Sentinel...")

	// 2. Load runtime configurations. Decoupling environment parameters from
	// the source code keeps deployment details modular and secure.
	config, err := database.LoadConfig("config.json")
	if err != nil {
		slog.Error("Critical Failure: Unable to parse config.json", "error", err)
		os.Exit(1)
	}

	// 3. Establish storage layer connection. Initializing tables and schema
	// guards upfront to guarantee data integrity before the application accepts traffic.
	slog.Info("Connecting to the shared suite storage layer...", "path", config.DatabasePath)
	db, err := database.InitDB(config.DatabasePath, config.SQLiteWALMode, "pkg/database/schema.sql")
	if err != nil {
		slog.Error("Critical Failure: Database pipeline collapse", "error", err)
		os.Exit(1)
	}

	// Pass config.SeedUsers down into the seed runner dynamically
	if err := database.SeedDefaultUsers(db, config.SeedUsers); err != nil {
		db.Close()
		slog.Error("Critical Failure: User database seeding failed", "error", err)
		os.Exit(1)
	}

	// 4. Initialize the decoupled Ingestion Engine. Passing traffic down a 10,000-capacity
	// buffered channel allows the API to return sub-2ms responses while background workers scale.
	alClient := metadata.NewAniListClient()
	engine := ingest.NewIngestionEngine(db, 10000, 4, alClient)
	engine.StartWorkerPool()

	// 5. Mount API route paths to Go's native HTTP multiplexer.
	mux := http.NewServeMux()

	// Lightweight endpoint for health checks and container liveness probes.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "💚 Anime Sentinel System Health: OPERATIONAL")
	})

	// Ingestion pipeline gateway for high-volume historical tracking data.
	mux.HandleFunc("POST /api/v1/ingest", engine.HandleIngest)

	// --- Analytical Intelligence Component ---
	intelEngine := intelligence.NewIntelligenceEngine(db)

	// Mathematical analytical endpoint to calculate consumption depth metrics.
	mux.HandleFunc("GET /api/v1/analytics/taste", intelEngine.HandleGetTasteAnchors)

	// 6. Configure the underlying HTTP Server wrapper explicitly to enable shutdown control.
	server := &http.Server{
		Addr:    config.Port,
		Handler: mux,
	}

	// 7. SETUP GRACEFUL SHUTDOWN INTERCEPTOR
	// Create a channel to listen for OS terminal interrupts (SIGINT = Ctrl+C, SIGTERM = Kill command)
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	// Ignite the server network socket inside an independent background routine so it doesn't block main.
	go func() {
		slog.Info("Network socket successfully bound", "port", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Critical Failure: Network socket server crashed", "error", err)
			os.Exit(1)
		}
	}()

	// --- THE PARKING GATE ---
	// Main execution pauses right here, waiting patiently until an OS shutdown signal drops into the channel!
	sig := <-shutdownSignal
	slog.Warn("Shutdown signal received! Initiating graceful pipeline teardown...", "signal", sig.String())

	// 8. TEARDOWN SEQUENCE (Executed in careful reverse order)

	// A. Stop accepting new API traffic immediately. Give active requests 5 seconds to finish processing.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP gateway server force-closed during teardown context", "error", err)
	} else {
		slog.Info("HTTP gateway server stopped successfully. Gateway locked.")
	}

	// B. Close the AniList client ticker to stop token allocations and release routines.
	slog.Info("Closing upstream metadata synchronization client...")
	alClient.Close()

	// C. Close the connection pool link to SQLite.
	// Since the pipeline is completely quiet, this forces a final WAL checkpoint,
	// flushes the -wal file logs, and collapses everything back into a single clean .db file!
	slog.Info("Flushing Write-Ahead Logs and closing state storage connection pool...")
	if err := db.Close(); err != nil {
		slog.Error("Error encountered while severing database pool connection", "error", err)
	} else {
		slog.Info("Database engine disconnected cleanly. All journal files collapsed!")
	}

	slog.Info("Anime Sentinel shutdown complete. System offline.")
}
