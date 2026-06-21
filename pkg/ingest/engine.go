package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Cfirth725/anime-sentinel/pkg/database"
	"github.com/Cfirth725/anime-sentinel/pkg/metadata"
	"github.com/Cfirth725/anime-sentinel/pkg/models"
	"github.com/Cfirth725/anime-sentinel/pkg/parser"
)

// IngestionEngine manages the thread-safe async buffer queue and orchestrates
// background database workers, decoupling the API from the storage layer.
type IngestionEngine struct {
	db          *sql.DB
	queue       chan models.IngestPayload
	workerCount int
	alClient    *metadata.AniListClient
}

// NewIngestionEngine constructs the core pipeline component, initializing the thread-safe
// buffered channel and binding the upstream synchronization client.
func NewIngestionEngine(db *sql.DB, bufferSize int, workerCount int, alClient *metadata.AniListClient) *IngestionEngine {
	return &IngestionEngine{
		db:          db,
		queue:       make(chan models.IngestPayload, bufferSize),
		workerCount: workerCount,
		alClient:    alClient,
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
// Incorporates a read-through cache layer to shield upstream API resources.
func (ie *IngestionEngine) worker(workerID int) {
	slog.Debug("Worker initialized and ready", "worker_id", workerID)

	for payload := range ie.queue {
		// 1. Execute title cleaning and normalization out-of-band
		media := parser.NormalizeWatchEntry(payload.RawTitle)

		// 2. Persist the raw normalized entry to the staging history table
		err := database.InsertIngestHistory(ie.db, payload.Username, media.BaseTitle, media.EpisodeNum, media.IsMovie, payload.Sentiment)
		if err != nil {
			slog.Error("Database insertion failed for staged payload item",
				"worker_id", workerID,
				"user", payload.Username,
				"error", err,
			)
			continue
		}

		slog.Debug("Worker successfully staged payload item to storage",
			"worker_id", workerID,
			"user", payload.Username,
			"normalized_title", media.BaseTitle,
			"episode", media.EpisodeNum,
		)

		// 3. READ-THROUGH CACHE: Check local storage before executing network calls
		cachedMedia, err := database.GetMediaByTitle(ie.db, media.BaseTitle)
		if err != nil {
			slog.Error("Cache layer lookup execution failure", "title", media.BaseTitle, "error", err)
			// Fall through to API if the cache layer safely errors out
		}

		if cachedMedia != nil {
			slog.Info("Cache HIT: Local metadata resolved. Bypassing upstream API.",
				"worker_id", workerID,
				"catalog_id", cachedMedia.ID,
				"title_romaji", cachedMedia.TitleRomaji,
				"format", cachedMedia.Format,
			)
			continue // SUCCESSFUL ⚡ SHORT-CIRCUIT: Pipeline task completed early!
		}

		// 4. Fetch third-party metadata from AniList via throttled client on cache miss
		aniListMedia, err := ie.alClient.FetchSeriesMetadata(media.BaseTitle)
		if err != nil {
			slog.Warn("Upstream external synchronization delayed or failed", "title", media.BaseTitle, "error", err)
			continue
		}

		if aniListMedia != nil {
			slog.Info("Cache POPULATE: Successfully synchronized tracking data with AniList API",
				"worker_id", workerID,
				"anilist_id", aniListMedia.ID,
				"title_romaji", aniListMedia.Title.Romaji,
			)

			// Safely extract the episode pointer into a standard int value
			totalEpisodes := 1
			if aniListMedia.Episodes != nil {
				totalEpisodes = *aniListMedia.Episodes
			}

			// 5. Commit the fresh API data to our local cache catalog for future worker lookups
			_, err = database.InsertMediaCatalog(
				ie.db,
				fmt.Sprintf("%d", aniListMedia.ID), // Convert int external ID to string matching schema
				aniListMedia.Title.Romaji,
				aniListMedia.Title.English,
				aniListMedia.Format,
				aniListMedia.Status,
				totalEpisodes,
			)
			if err != nil {
				slog.Error("Failed to write fresh upstream metadata to local cache storage", "title", media.BaseTitle, "error", err)
			}
		} else {
			slog.Warn("Upstream match execution completed with zero results", "title", media.BaseTitle)
		}
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
