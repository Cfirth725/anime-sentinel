package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
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
	idlePrinted uint32
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
		// 1. DATA VERIFICATION (Using the refactored regex engine to normalize titles)
		parsedMedia := parser.NormalizeWatchEntry(payload.SeriesTitle)
		title := parsedMedia.BaseTitle
		episode := payload.EpisodeNumber

		// 2. Persist the raw normalized entry to the staging history table
		query := `
			INSERT INTO ingest_staging_history (
				username, series_id, series_title, season_number, 
				episode_number, episode_title, episode_id, watched_at, fully_watched, sentiment, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP);
		`
		_, err := ie.db.Exec(query,
			payload.Username,
			payload.SeriesID,
			title,
			payload.SeasonNumber,
			episode,
			payload.EpisodeTitle,
			payload.EpisodeID,
			payload.WatchedAt,
			payload.FullyWatched,
			payload.Sentiment,
		)
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
			"normalized_title", title,
			"episode", episode,
		)

		// 3. USER IDENTITY RESOLUTION: Fetch the user profile row ID
		user, err := database.GetUserByUsername(ie.db, payload.Username)
		if err != nil {
			slog.Error("User identification resolution error", "username", payload.Username, "error", err)
			continue
		}
		if user == nil {
			slog.Warn("Rejected ingestion processing: User profile does not exist in local database", "username", payload.Username)
			continue
		}

		// Keep track of our relational media row ID across cache hits or misses
		var localMediaID int64

		// 4. READ-THROUGH CACHE: Check local storage before executing network calls
		cachedMedia, err := database.GetMediaByTitle(ie.db, title)
		if err != nil {
			slog.Error("Cache layer lookup execution failure", "title", title, "error", err)
		}

		if cachedMedia != nil {
			slog.Info("Cache HIT: Local metadata resolved. Bypassing upstream API.",
				"worker_id", workerID,
				"catalog_id", cachedMedia.ID,
				"title_romaji", cachedMedia.TitleRomaji,
			)
			localMediaID = cachedMedia.ID
		} else {
			slog.Debug("Cache MISS: Title not found in local catalog. Queueing for API.", "title", title)

			// 5. Fetch third-party metadata from AniList via throttled client on cache miss
			aniListMedia, err := ie.alClient.FetchSeriesMetadata(title)

			// --- LOCK-FREE DOUBLE CHECK LAYER ---
			// Now that the throttling token has been acquired, check if a concurrent sibling
			// routine successfully resolved the cache while this thread was standing in line.
			doubleCheckMedia, checkErr := database.GetMediaByTitle(ie.db, title)
			if checkErr == nil && doubleCheckMedia != nil {
				slog.Info("Cache STAMPEDE MITIGATED: Local metadata resolved post-token. Bypassing API execution.",
					"worker_id", workerID,
					"catalog_id", doubleCheckMedia.ID,
					"title_romaji", doubleCheckMedia.TitleRomaji,
				)
				localMediaID = doubleCheckMedia.ID
				goto updateProgress // Jump over network execution and insertion directly to status tracking
			}
			// ------------------------------------

			if err != nil {
				slog.Warn("Upstream external synchronization delayed or failed", "title", title, "error", err)

				// ONLY sleep if it's a 429 Rate Limit block
				if strings.Contains(err.Error(), "429") {
					slog.Info("Rate limit hit. Pacing worker queue consumption...", "worker_id", workerID)
					time.Sleep(5 * time.Second)
				}
				continue
			}

			if aniListMedia != nil {
				slog.Info("Cache POPULATE: Successfully synchronized tracking data with AniList API",
					"worker_id", workerID,
					"anilist_id", aniListMedia.ID,
					"title_romaji", aniListMedia.Title.Romaji,
				)

				totalEpisodes := 1
				if aniListMedia.Episodes != nil {
					totalEpisodes = *aniListMedia.Episodes
				}

				// 6. Commit the fresh API data to our local cache catalog
				insertedID, err := database.InsertMediaCatalog(
					ie.db,
					fmt.Sprintf("%d", aniListMedia.ID),
					title,
					aniListMedia.Title.Romaji,
					aniListMedia.Title.English,
					aniListMedia.Format,
					aniListMedia.Status,
					totalEpisodes,
				)
				if err != nil {
					slog.Error("Failed to write fresh upstream metadata to local cache storage", "title", title, "error", err)
					continue
				}
				localMediaID = insertedID
			} else {
				slog.Warn("Upstream match execution completed with zero results", "title", title)
				continue
			}
		}

	updateProgress: // Label anchor pointing directly to user watch record tracking
		// 7. STATE ENGINE UPDATE: Progressively upsert running user watch tracking metrics
		err = database.UpsertWatchProgress(ie.db, user.ID, localMediaID, episode, payload.Sentiment)
		if err != nil {
			slog.Error("Failed to commit tracking checkpoint to watch_progress ledger", "username", user.Username, "error", err)
			continue
		}

		slog.Info("Progress state updated successfully",
			"username", user.Username,
			"title", title,
			"episode", episode,
		)

		// SOLID SINGLE-LINE SIGNAL FOR BATCH RUN COMPLETION
		if len(ie.queue) == 0 {
			if atomic.CompareAndSwapUint32(&ie.idlePrinted, 0, 1) {
				fmt.Println("\n\033[1;32m================ MIGRATION COMPLETELY FINISHED ================\033[0m")
				slog.Info("Pipeline idle state achieved.")
			}
		} else {
			atomic.StoreUint32(&ie.idlePrinted, 0)
		}
	}
}

// HandleIngest serves as the high-performance HTTP gateway loop. It decodes batches,
// runs rapid sanity checks, extracts tracking headers, and offloads payloads to the channel queue.
func (ie *IngestionEngine) HandleIngest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Enforce strict routing restrictions at the gateway level
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract context attributes from URL query strings
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "Missing required 'username' query parameter", http.StatusBadRequest)
		return
	}

	sentimentVal := 0
	if sentimentParam := r.URL.Query().Get("sentiment"); sentimentParam != "" {
		fmt.Sscanf(sentimentParam, "%d", &sentimentVal)
	}

	// Decode the wrapped JSON payload envelope
	var envelope models.CrunchyrollImportEnvelope
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&envelope); err != nil {
		slog.Error("Failed to decode enveloped ingestion batch", "error", err)
		http.Error(w, "Invalid JSON structure", http.StatusBadRequest)
		return
	}

	var acceptedCount int
	// Loop through the inner episodes array directly!
	for _, p := range envelope.Episodes {
		// Inject gateway context into each row model dynamically
		p.Username = username
		p.Sentiment = sentimentVal

		if p.SeriesTitle == "" {
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
		"received": len(envelope.Episodes),
		"queued":   acceptedCount,
		"elapsed":  duration.String(),
	})
}
