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
	// Initialize structured logging. Using slog key-value pairs ensures
	// the application telemetry is machine-readable and ready for log aggregators.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	slog.Info("Launching Anime Sentinel...")

	// Load runtime configurations. Decoupling environment parameters from
	// the source code keeps deployment details modular and secure.
	config, err := database.LoadConfig("config.json")
	if err != nil {
		slog.Error("Critical Failure: Unable to parse config.json", "error", err)
		os.Exit(1)
	}

	// Establish storage layer connection. Initializing tables and schema
	// guards upfront to guarantee data integrity before the application accepts traffic.
	slog.Info("Connecting to the shared suite storage layer...", "path", config.DatabasePath)
	db, err := database.InitDB(config.DatabasePath, config.SQLiteWALMode, "pkg/database/schema.sql")
	if err != nil {
		slog.Error("Critical Failure: Database pipeline collapse", "error", err)
		os.Exit(1)
	}

	// Dynamic dependency migration pass to seed structural environment user keys.
	if err := database.SeedDefaultUsers(db, config.SeedUsers); err != nil {
		db.Close()
		slog.Error("Critical Failure: User database seeding failed", "error", err)
		os.Exit(1)
	}

	// Instantiate the background metadata synchronization client.
	alClient := metadata.NewAniListClient()

	// Initialize the decoupled Ingestion Engine. Passing traffic down a 10,000-capacity
	// buffered channel enables sub-2ms response cycles while background routines drain the queue.
	engine := ingest.NewIngestionEngine(db, 10000, 4, alClient)
	engine.StartWorkerPool()

	// Initialize the analytical computation layer to track engagement metrics,
	// isolate taste profiles, and cross-reference shared user viewing habits.
	intelEngine := intelligence.NewIntelligenceEngine(db, alClient)

	// Register API routing patterns directly to Go's native HTTP multiplexer.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/ingest", engine.HandleIngest)
	mux.HandleFunc("GET /api/v1/analytics/taste", intelEngine.HandleGetTasteAnchors)
	mux.HandleFunc("GET /api/v1/analytics/shared", intelEngine.HandleGetSharedRecommendations)

	// Lightweight endpoint for health checks and container liveness probes.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "💚 Anime Sentinel System Health: OPERATIONAL")
	})

	// Configure the underlying HTTP Server wrapper to support controlled lifecycle shutdowns.
	server := &http.Server{
		Addr:    config.Port,
		Handler: mux,
	}

	// Listen explicitly for operating system lifecycle terminal interrupts.
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	// Ignite the network listener socket in a non-blocking background routine.
	go func() {
		slog.Info("Network socket successfully bound", "port", config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Critical Failure: Network socket server crashed", "error", err)
			os.Exit(1)
		}
	}()

	// Execution pauses here, unblocking only when an active OS signal drops into the channel.
	sig := <-shutdownSignal
	slog.Warn("Shutdown signal received! Initiating graceful pipeline teardown...", "signal", sig.String())

	// Execute the lifecycle teardown sequence in strict reverse order of instantiation.
	// 1. Force the gateway to stop accepting new connection threads and drain active requests.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP gateway server force-closed during teardown context", "error", err)
	} else {
		slog.Info("HTTP gateway server stopped successfully. Gateway locked.")
	}

	// 2. Shut down the rate limiter clock to release blocked goroutines and prevent resource leaks.
	slog.Info("Closing upstream metadata synchronization client...")
	alClient.Close()

	// 3. Sever connection links to the storage layer, forcing a final database WAL checkpoint
	// to cleanly collapse active journal files back into the core file on disk.
	slog.Info("Flushing Write-Ahead Logs and closing state storage connection pool...")
	if err := db.Close(); err != nil {
		slog.Error("Error encountered while severing database pool connection", "error", err)
	} else {
		slog.Info("Database engine disconnected cleanly. All journal files collapsed!")
	}

	slog.Info("Anime Sentinel shutdown complete. System offline.")
}
