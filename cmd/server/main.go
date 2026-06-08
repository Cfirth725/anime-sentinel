package main

import (
	"fmt"
	"github.com/Cfirth725/anime-sentinel/pkg/database"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	// 1. Initialize a clean structured logging instance
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("🚀 Launching Anime Sentinel (The Sentinel Suite)...")

	// 2. Ingest the local non-tracked environment parameters
	config, err := database.LoadConfig("config.json")
	if err != nil {
		slog.Error("❌ Critical Failure: Unable to parse config.json", "error", err)
		os.Exit(1)
	}

	// 3. Fire up the data layer and execute the decoupled schema.sql file
	slog.Info("💾 Connecting to the shared suite storage layer...", "path", config.DatabasePath)
	db, err := database.InitDB(config.DatabasePath, config.SQLiteWALMode, "pkg/database/schema.sql")
	if err != nil {
		slog.Error("❌ Critical Failure: Database pipeline collapse", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 4. Stand up a placeholder HTTP route to verify network binding
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "💚 Anime Sentinel System Health: OPERATIONAL")
	})

	slog.Info("📡 Network socket successfully bound", "port", config.Port)

	// 5. Spin up the native HTTP network engine
	if err := http.ListenAndServe(config.Port, mux); err != nil {
		slog.Error("❌ Critical Failure: Network socket server crashed", "error", err)
		os.Exit(1)
	}
}
