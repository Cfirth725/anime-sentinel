// Package database handles configuration loading, local storage lifecycle initialization,
// database seeding, and transactional ledger reads/writes for user tracking state.
package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/Cfirth725/anime-sentinel/pkg/models"
	_ "github.com/mattn/go-sqlite3"
)

// ====================================================================
//         -- SUBSYSTEM CONFIGURATION & BOOTSTRAPPING ENGINE --
// ====================================================================

// Config encapsulates environment configuration fields mapped to JSON file properties.
type Config struct {
	Port          string   `json:"PORT"`
	DatabasePath  string   `json:"DATABASE_PATH"`
	SQLiteWALMode bool     `json:"SQLITE_WAL_MODE"`
	SeedUsers     []string `json:"SEED_USERS"`
}

// LoadConfig opens and decodes an untracked local JSON parameter file.
// Utilizing a stream decoder minimizes allocations compared to loading the entire file into memory.
func LoadConfig(path string) (Config, error) {
	var config Config
	file, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	return config, err
}

// InitDB initializes the SQLite connection pool and handles schema parsing from an external file.
// It explicitly configures WAL mode if requested to maximize database write throughput.
func InitDB(dbPath string, useWAL bool, schemaFilePath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// WAL mode decouples reader and writer transactions to maximize database throughput.
	if useWAL {
		if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
		}
		slog.Info("[INIT] SQLite engine initialized with Write-Ahead Logging (WAL)")
	}

	schemaBytes, err := os.ReadFile(schemaFilePath)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to read schema file at %s: %w", schemaFilePath, err)
	}

	if _, err := db.Exec(string(schemaBytes)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute external schema sql: %w", err)
	}

	slog.Info("[INIT] Database schema and indexing verified successfully")
	return db, nil
}

// SeedDefaultUsers checks if the users table is empty and injects initial profiles
// defined within the external configuration parameters.
func SeedDefaultUsers(db *sql.DB, userList []string) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users;").Scan(&count); err != nil {
		return fmt.Errorf("failed to check existing user count: %w", err)
	}

	if count > 0 {
		return nil
	}

	if len(userList) == 0 {
		slog.Warn("[INIT] No bootstrap users defined in configuration. Skipping seeding step.")
		return nil
	}

	slog.Info("[INIT] Base user profiles not detected. Seeding default suite accounts...")
	query := `INSERT INTO users (username, created_at) VALUES (?, CURRENT_TIMESTAMP);`

	for _, username := range userList {
		if _, err := db.Exec(query, username); err != nil {
			return fmt.Errorf("failed to seed user profile [%s]: %w", username, err)
		}
		slog.Info("[OK] User profile successfully bootstrapped", "username", username)
	}

	return nil
}

// ====================================================================
//                -- DATA ACCESS OBJECTS (DAO LAYER) --
// ====================================================================

// GetUserByUsername resolves a raw string username to its corresponding structural profile row.
// It uses a case-insensitive lookup to guarantee loose name matching safety.
func GetUserByUsername(db *sql.DB, username string) (*models.User, error) {
	query := `SELECT id, username, created_at FROM users WHERE LOWER(username) = LOWER(?);`

	var u models.User
	var createdAtStr string

	err := db.QueryRow(query, username).Scan(&u.ID, &u.Username, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user identity profile resolution failure: %w", err)
	}
	return &u, nil
}

// InsertIngestHistory inserts a rich, normalized tracking payload into the staging history table.
func InsertIngestHistory(db *sql.DB, username string, title string, episode float64, sentiment int) error {
	query := `
		INSERT INTO anime_ingest_staging_history (
			username, series_id, series_title, season_number, 
			episode_number, episode_title, episode_id, watched_at, fully_watched, sentiment, created_at
		) VALUES (?, 'GENERIC', ?, 1, ?, 'Imported Item', 'GENERIC', CURRENT_TIMESTAMP, 1, ?, CURRENT_TIMESTAMP);`

	_, err := db.Exec(query, username, title, episode, sentiment)
	return err
}

// GetMediaByTitle queries the anime_catalog cache using case-insensitive loose title matching.
// Returns a pointer to models.AnimeCatalog, or nil if no local cache hit is scored.
func GetMediaByTitle(db *sql.DB, cleanTitle string) (*models.AnimeCatalog, error) {
	query := `
		SELECT 
			ac.id, 
			ac.external_id, 
			ac.title_romaji, 
			ac.title_english, 
			ac.format, 
			ac.status, 
			COALESCE(ad.total_episodes_count, 1), 
			ac.updated_at
		FROM anime_catalog ac
		LEFT JOIN anime_catalog_depths ad ON ac.id = ad.anime_id
		WHERE LOWER(ac.cache_key) = LOWER(?) OR LOWER(ac.title_romaji) = LOWER(?) OR LOWER(ac.title_english) = LOWER(?)
		LIMIT 1;`

	var m models.AnimeCatalog
	var updatedAtStr string

	err := db.QueryRow(query, cleanTitle, cleanTitle, cleanTitle).Scan(
		&m.ID,
		&m.ExternalID,
		&m.TitleRomaji,
		&m.TitleEnglish,
		&m.Format,
		&m.Status,
		&m.TotalEpisodesCount,
		&updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("local anime catalog cache lookup failed: %w", err)
	}

	return &m, nil
}

// InsertMediaCatalog populates the localized anime cache layer with data fetched from upstream.
// It performs a transactional upsert to both anime_catalog (metadata) and anime_catalog_depths (episodic limits),
// returning the verified internal row ID of the catalog asset.
func InsertMediaCatalog(db *sql.DB, externalID string, cacheKey string, romaji string, english string, format string, status string, episodes int) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin catalog transaction: %w", err)
	}
	defer tx.Rollback() // Safe to defer; no-op if the transaction is committed

	// Step A: Upsert metadata into anime_catalog
	catalogQuery := `
		INSERT INTO anime_catalog (external_id, cache_key, title_romaji, title_english, format, status, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(external_id) DO UPDATE SET
			cache_key = COALESCE(anime_catalog.cache_key, excluded.cache_key),
			title_romaji = excluded.title_romaji,
			title_english = excluded.title_english,
			status = excluded.status,
			updated_at = CURRENT_TIMESTAMP;`

	if _, err := tx.Exec(catalogQuery, externalID, cacheKey, romaji, english, format, status); err != nil {
		return 0, fmt.Errorf("failed to upsert anime_catalog row metadata: %w", err)
	}

	// Step B: Retrieve the row ID of the catalog asset
	var actualID int64
	idLookupQuery := `SELECT id FROM anime_catalog WHERE external_id = ? LIMIT 1;`
	if err := tx.QueryRow(idLookupQuery, externalID).Scan(&actualID); err != nil {
		return 0, fmt.Errorf("failed to retrieve row id during transactional insert: %w", err)
	}

	// Step C: Upsert the episodic depth bounds into anime_catalog_depths
	depthQuery := `
		INSERT INTO anime_catalog_depths (anime_id, total_episodes_count)
		VALUES (?, ?)
		ON CONFLICT(anime_id) DO UPDATE SET
			total_episodes_count = excluded.total_episodes_count;`

	if _, err := tx.Exec(depthQuery, actualID, episodes); err != nil {
		return 0, fmt.Errorf("failed to upsert anime_catalog_depths row: %w", err)
	}

	// Step D: Commit the complete changes to disk safely
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit catalog transaction: %w", err)
	}

	return actualID, nil
}

// UpsertWatchProgress updates a user's running checkpoint progress ledger for a given media asset.
// It will only advance the current episode counter if the incoming episode number is greater
// than the recorded value, protecting against out-of-order stream processing.
func UpsertWatchProgress(db *sql.DB, userID int64, mediaID int64, episodeNum float64, sentiment int) error {
	query := `
		INSERT INTO anime_watch_progress (user_id, anime_id, current_episode_progress, last_watched_at, sentiment)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(user_id, anime_id) DO UPDATE SET
			current_episode_progress = MAX(anime_watch_progress.current_episode_progress, excluded.current_episode_progress),
			sentiment = excluded.sentiment,
			last_watched_at = CURRENT_TIMESTAMP;`

	if _, err := db.Exec(query, userID, mediaID, episodeNum, sentiment); err != nil {
		return fmt.Errorf("failed to upsert watch progress tracker state: %w", err)
	}
	return nil
}

// GetUserEngagementProfiles computes the mathematical engagement depth index for all
// media entries bound to a specific user identity, flagging taste anchors dynamically.
func GetUserEngagementProfiles(db *sql.DB, userID int64) ([]models.UserEngagement, error) {
	query := `
		SELECT 
			ac.id,
			ac.title_romaji,
			wp.current_episode_progress,
			COALESCE(ad.total_episodes_count, 1)
		FROM anime_watch_progress wp
		JOIN anime_catalog ac ON wp.anime_id = ac.id
		LEFT JOIN anime_catalog_depths ad ON ac.id = ad.anime_id
		WHERE wp.user_id = ?;`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute engagement profile query: %w", err)
	}
	defer rows.Close()

	var profiles []models.UserEngagement

	for rows.Next() {
		var prof models.UserEngagement
		var currentProgress float64

		if err := rows.Scan(&prof.AnimeID, &prof.BaseTitle, &currentProgress, &prof.TotalEpisodes); err != nil {
			return nil, fmt.Errorf("failed to scan engagement row: %w", err)
		}

		prof.EpisodesWatched = int(currentProgress)

		// ANOMALY GUARD: Prevent division by zero and cap upper bound to 100%
		if prof.TotalEpisodes > 0 {
			computedScore := (float64(prof.EpisodesWatched) / float64(prof.TotalEpisodes)) * 100.0
			if computedScore > 100.0 {
				prof.Score = 100.0
			} else {
				prof.Score = computedScore
			}
		} else {
			prof.Score = 0.0
		}

		// Calculate threshold state dynamically based on the 80% engagement rule
		if prof.Score >= 80.0 {
			prof.IsTasteAnchor = true
		}

		profiles = append(profiles, prof)
	}

	return profiles, nil
}
