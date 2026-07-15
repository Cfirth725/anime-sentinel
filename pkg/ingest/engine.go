// Package ingest manages asynchronous worker pools, background task queuing,
// and transactional data streaming gates for media payload synchronization.
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

// ====================================================================
//                -- ENGINE STRUCT & INITIALIZATION --
// ====================================================================

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
	slog.Info("[INIT] Activating asynchronous worker pool...", "workers", ie.workerCount)
	for i := 1; i <= ie.workerCount; i++ {
		go ie.worker(i)
	}
}

// ====================================================================
//             -- BACKGROUND ASYNC ROUTINE WORKER POOLS --
// ====================================================================

// worker represents an autonomous background consumer method that continuously drains the central channel.
// It incorporates a lock-free double-check read-through cache layer to shield upstream API resources.
func (ie *IngestionEngine) worker(workerID int) {
	slog.Debug("[INIT] Background routine worker pool listener initialized", "worker_id", workerID)

	for payload := range ie.queue {
		parsedMedia := parser.NormalizeWatchEntry(payload.SeriesTitle)
		title := parsedMedia.BaseTitle
		episode := payload.EpisodeNumber

		query := `
			INSERT INTO anime_ingest_staging_history (
				username, series_id, series_title, season_number, 
				episode_number, episode_title, episode_id, watched_at, fully_watched, sentiment, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP);`

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
			slog.Error("[ERROR] Local staging ledger persistence failure",
				"worker_id", workerID,
				"user", payload.Username,
				"error", err,
			)
			continue
		}

		slog.Debug("[REALTIME] Stream payload entry staged successfully to history logs",
			"worker_id", workerID,
			"user", payload.Username,
			"normalized_title", title,
			"episode", episode,
		)

		user, err := database.GetUserByUsername(ie.db, payload.Username)
		if err != nil {
			slog.Error("[ERROR] Core profile lookup failure", "username", payload.Username, "error", err)
			continue
		}
		if user == nil {
			slog.Warn("[ERROR] Action rejected: Target username profile missing from local engine database", "username", payload.Username)
			continue
		}

		var localMediaID int64

		cachedMedia, err := database.GetMediaByTitle(ie.db, title)
		if err != nil {
			slog.Error("[ERROR] Database cache lookup runtime failure", "title", title, "error", err)
		}

		if cachedMedia != nil {
			slog.Info("[REALTIME] Cache HIT: Storage catalog cache verified. Bypassing upstream networking.",
				"worker_id", workerID,
				"catalog_id", cachedMedia.ID,
				"title", cachedMedia.TitleRomaji,
			)
			localMediaID = cachedMedia.ID
		} else {
			slog.Debug("[REALTIME] Cache MISS: Target missing from local catalog. Fetching external allocation.", "title", title)

			ie.alClient.AcquireToken()

			doubleCheckMedia, checkErr := database.GetMediaByTitle(ie.db, title)
			if checkErr == nil && doubleCheckMedia != nil {
				slog.Info("[REALTIME] Cache STAMPEDE MITIGATED: Catalog populated post-token. Network call aborted.",
					"worker_id", workerID,
					"catalog_id", doubleCheckMedia.ID,
					"title", doubleCheckMedia.TitleRomaji,
				)
				localMediaID = doubleCheckMedia.ID
				goto updateProgress
			}

			aniListMedia, err := ie.alClient.FetchSeriesMetadata(title)
			if err != nil {
				slog.Warn("[REALTIME] Upstream metadata synchronization network request dropped", "title", title, "error", err)

				if strings.Contains(err.Error(), "429") {
					slog.Info("[REALTIME] Backoff signal triggered. Intercepting worker queue pace...", "worker_id", workerID)
					time.Sleep(5 * time.Second)
				}
				continue
			}

			if aniListMedia != nil {
				slog.Info("[REALTIME] Cache POPULATE: Outbound directory synchronization successful",
					"worker_id", workerID,
					"anilist_id", aniListMedia.ID,
					"title_romaji", aniListMedia.Title.Romaji,
				)

				totalEpisodes := 1
				if aniListMedia.Episodes != nil {
					totalEpisodes = *aniListMedia.Episodes
				}

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
					slog.Error("[ERROR] Cache mapping persistence layer fault", "title", title, "error", err)
					continue
				}
				localMediaID = insertedID
			} else {
				slog.Warn("[REALTIME] External match pass returned clean zero bounds", "title", title)
				continue
			}
		}

	updateProgress:
		err = database.UpsertWatchProgress(ie.db, user.ID, localMediaID, episode, payload.Sentiment)
		if err != nil {
			slog.Error("[ERROR] User state tracking engine checkpoint failure", "username", user.Username, "error", err)
			continue
		}

		slog.Info("[OK] Progress tracker update transaction committed",
			"username", user.Username,
			"title", title,
			"episode", episode,
		)

		if len(ie.queue) == 0 {
			if atomic.CompareAndSwapUint32(&ie.idlePrinted, 0, 1) {
				fmt.Println("\n\033[1;32m================ MIGRATION COMPLETELY FINISHED ================\033[0m")
				slog.Info("[IDLE] Processing stream exhausted. Pipeline idle state achieved.")
			}
		} else {
			atomic.StoreUint32(&ie.idlePrinted, 0)
		}
	}
}

// ====================================================================
//                -- HTTP PUBLIC ENTRYWAYS & GATES --
// ====================================================================

// HandleIngest serves as the high-performance HTTP gateway loop. It decodes batches,
// runs rapid sanity checks, extracts tracking headers, and offloads payloads to the channel queue.
func (ie *IngestionEngine) HandleIngest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "Missing required 'username' query parameter", http.StatusBadRequest)
		return
	}

	sentimentVal := 0
	if sentimentParam := r.URL.Query().Get("sentiment"); sentimentParam != "" {
		fmt.Sscanf(sentimentParam, "%d", &sentimentVal)
	}

	var envelope models.CrunchyrollImportEnvelope
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&envelope); err != nil {
		slog.Error("[SERVER] JSON stream decoding fault on payload envelope drop", "error", err)
		http.Error(w, "Invalid JSON structure", http.StatusBadRequest)
		return
	}

	var acceptedCount int
	for _, p := range envelope.Episodes {
		p.Username = username
		p.Sentiment = sentimentVal

		if p.SeriesTitle == "" {
			continue
		}

		select {
		case ie.queue <- p:
			acceptedCount++
		default:
			slog.Error("[ERROR] Resource bottleneck: Async core buffer channel max capacity hit. Dropping incoming packets.")
			http.Error(w, "Server Resource Saturation: Storage Buffer Exhausted", http.StatusServiceUnavailable)
			return
		}
	}

	duration := time.Since(start)
	slog.Info("[SERVER] Batch ingestion intake dispatch successful", "count", acceptedCount, "duration", duration)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "accepted",
		"received": len(envelope.Episodes),
		"queued":   acceptedCount,
		"elapsed":  duration.String(),
	}); err != nil {
		slog.Error("[SERVER] Gateway serialization execution fault", "error", err)
	}
}
