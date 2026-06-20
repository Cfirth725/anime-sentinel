package ingest

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/Cfirth725/anime-sentinel/pkg/models"
	"github.com/Cfirth725/anime-sentinel/pkg/parser"
)

// IngestionEngine manages the thread-safe async buffer queue and orchestrates
// background database workers, decoupling the API from the storage layer.
type IngestionEngine struct {
	db          *sql.DB
	queue       chan models.IngestPayload
	workerCount int
}

// NewIngestionEngine constructs the core pipeline component, initializing the thread-safe
// buffered channel to balance maximum payload capacity against the system memory footprint.
func NewIngestionEngine(db *sql.DB, bufferSize int, workerCount int) *IngestionEngine {
	return &IngestionEngine{
		db:          db,
		queue:       make(chan models.IngestPayload, bufferSize),
		workerCount: workerCount,
	}
}

// StartWorkerPool ignites the background routines. Using a pointer receiver (*IngestionEngine)
// ensures reference to the engine's active channel instead of duplicating it in memory.
func (ie *IngestionEngine) StartWorkerPool() {
	slog.Info("Activating asynchronous worker pool...", "workers", ie.workerCount)
	for i := 1; i <= ie.workerCount; i++ {
		go ie.worker(i) // Spawn autonomous background threads via goroutines
	}
}

// Background worker method that continuously drains the central channel.
// Running concurrently allows heavy data translations to execute without blocking the client.
func (ie *IngestionEngine) worker(workerID int) {
	slog.Debug("Worker initialized and ready", "worker_id", workerID)

	// Range over the channel continuously drains items until the channel is explicitly closed.
	for payload := range ie.queue {
		// Execute title cleaning and normalization logic out-of-band.
		// Captures the single NormalizedMedia struct returned by the parser.
		media := parser.NormalizeWatchEntry(payload.RawTitle)

		slog.Info("Worker processed and normalized payload item",
			"worker_id", workerID,
			"user", payload.Username,
			"raw_title", payload.RawTitle,
			"normalized_title", media.BaseTitle,
			"episode", media.EpisodeNum,
			"is_movie", media.IsMovie,
			"sentiment", payload.Sentiment,
		)

		// Temporary baseline throttle to simulate storage and network latency.
		// This will be replaced by Step 2 database insertions.
		time.Sleep(50 * time.Millisecond)
	}
}

// HandleIngest serves as the high-performance HTTP gateway loop. It decodes batches,
// runs rapid sanity checks, and offloads payloads to the channel queue in under 2ms.
func (ie *IngestionEngine) HandleIngest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Enforce strict routing restrictions at the gateway level
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Stream decode the raw array payload directly into memory models to optimize allocation
	var payloads []models.IngestPayload
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&payloads); err != nil {
		slog.Error("Failed to decode ingestion array batch", "error", err)
		http.Error(w, "Invalid JSON structure", http.StatusBadRequest)
		return
	}

	var acceptedCount int
	for _, p := range payloads {
		// Validate data boundaries rapidly before pushing data deeper into the ecosystem
		if p.Sentiment < -1 || p.Sentiment > 1 {
			slog.Warn("Rejected individual payload item out-of-bounds sentiment", "sentiment", p.Sentiment)
			continue
		}
		if p.RawTitle == "" || p.Username == "" {
			slog.Warn("Rejected malformed payload tracking context missing fields")
			continue
		}

		// Non-blocking channel handoff. The select block attempts a direct push; if the
		// buffer is completely full, it triggers the default fallback instantly to prevent deadlocks.
		select {
		case ie.queue <- p:
			acceptedCount++
		default:
			slog.Error("Critical Alert: Buffer channel full! Ingestion dropping payloads.")
			http.Error(w, "Server Resource Saturation: Storage Buffer Exhausted", http.StatusServiceUnavailable)
			return
		}
	}

	// Calculate execution duration metrics to track ingestion pipeline efficiency
	duration := time.Since(start)
	slog.Info("Ingestion bulk dispatch successful", "count", acceptedCount, "duration", duration)

	// Return a 202 Accepted status code to explicitly signal that the data has been
	// securely queued for processing, allowing the client to disconnect immediately.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "accepted",
		"received": len(payloads),
		"queued":   acceptedCount,
		"elapsed":  duration.String(),
	})
}
