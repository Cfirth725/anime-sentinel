package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/Cfirth725/anime-sentinel/pkg/database"
	"github.com/Cfirth725/anime-sentinel/pkg/ingest"
)

func main() {
	// 1. Initialize structured text logging. Using slog key-value pairs ensures 
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
	defer db.Close()

	// 4. Initialize the decoupled Ingestion Engine. Passing traffic down a 10,000-capacity 
	// buffered channel allows the API to return sub-2ms responses while background workers scale.
	engine := ingest.NewIngestionEngine(db, 10000, 4)
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

	slog.Info("Network socket successfully bound", "port", config.Port)

	// 6. Ignite the native HTTP server network socket. This creates a blocking process 
	// that continuously listens for incoming client connections.
	if err := http.ListenAndServe(config.Port, mux); err != nil {
		slog.Error("Critical Failure: Network socket server crashed", "error", err)
		os.Exit(1)
	}
}
