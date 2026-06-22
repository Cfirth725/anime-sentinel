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

// Config encapsulates environment configuration fields mapped to JSON file properties.
type Config struct {
	Port          string   `json:"PORT"`
	DatabasePath  string   `json:"DATABASE_PATH"`
	SQLiteWALMode bool     `json:"SQLITE_WAL_MODE"`
	SeedUsers     []string `json:"SEED_USERS"`
}

// LoadConfig opens and decodes an un-tracked local JSON parameter file.
// Utilizing a stream decoder minimizes allocations compared to loading the entire file into an intermediate buffer.
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
func InitDB(dbPath string, useWAL bool, schemaFilePath string) (*sql.DB, error) {
	// 1. Establish the database engine connection pool link.
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// 2. Configure high-concurrency multi-process Write-Ahead Logging (WAL) settings.
	// WAL mode decouples reader and writer transactions to maximize database throughput.
	if useWAL {
		if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
		}
		slog.Info("SQLite engine initialized with Write-Ahead Logging (WAL)")
	}

	// 3. Read the external schema.sql file definition into memory.
	schemaBytes, err := os.ReadFile(schemaFilePath)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to read schema file at %s: %w", schemaFilePath, err)
	}

	// 4. Execute the raw schema SQL statements to verify database partitions and constraints.
	if _, err := db.Exec(string(schemaBytes)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute external schema sql: %w", err)
	}

	slog.Info("Database schema and indexing verified successfully")
	return db, nil
}

// InsertIngestHistory inserts a normalized tracking payload into the staging history table.
// Using explicit argument bounds enforces data integrity at the database layer.
func InsertIngestHistory(db *sql.DB, username string, baseTitle string, episodeNum int, isMovie bool, sentiment int) error {
	query := `
		INSERT INTO ingest_staging_history (username, normalized_title, episode_number, is_movie, sentiment, created_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP);
	`

	// Convert the boolean isMovie flag to an integer representation for SQLite storage compatibility
	isMovieFlag := 0
	if isMovie {
		isMovieFlag = 1
	}

	_, err := db.Exec(query, username, baseTitle, episodeNum, isMovieFlag, sentiment)
	return err
}

// GetMediaByTitle queries the media_catalog cache using case-insensitive loose title matching.
// Returns a nil pointer and nil error if the title does not exist in the local storage layer.
func GetMediaByTitle(db *sql.DB, cleanTitle string) (*models.MediaCatalog, error) {
	query := `
		SELECT id, external_id, title_romaji, title_english, format, status, total_episodes_count, updated_at
		FROM media_catalog
		WHERE LOWER(search_query) = LOWER(?) OR LOWER(title_romaji) = LOWER(?) OR LOWER(title_english) = LOWER(?)
		LIMIT 1;
	`

	var m models.MediaCatalog
	var updatedAtStr string // Intermediate string scanner for SQLite datetime conversion

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
		return nil, nil // Cache miss: Record safely not found
	}
	if err != nil {
		return nil, fmt.Errorf("local media catalog cache lookup failed: %w", err)
	}

	return &m, nil // Cache hit
}

// InsertMediaCatalog populates the localized media cache layer with data fetched from upstream.
func InsertMediaCatalog(db *sql.DB, externalID string, searchQuery string, romaji string, english string, format string, status string, episodes int) (int64, error) {
	query := `
		INSERT INTO media_catalog (external_id, search_query, title_romaji, title_english, format, status, total_episodes_count, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(external_id) DO UPDATE SET
			search_query = COALESCE(media_catalog.search_query, excluded.search_query),
			title_romaji = excluded.title_romaji,
			title_english = excluded.title_english,
			status = excluded.status,
			total_episodes_count = excluded.total_episodes_count,
			updated_at = CURRENT_TIMESTAMP;
	`

	res, err := db.Exec(query, externalID, searchQuery, romaji, english, format, status, episodes)
	if err != nil {
		return 0, fmt.Errorf("failed to commit metadata payload to media_catalog: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve local insertion row id: %w", err)
	}

	return id, nil
}

// GetUserByUsername resolves a raw string username to its corresponding structural profile row.
func GetUserByUsername(db *sql.DB, username string) (*models.User, error) {
	query := `SELECT id, username, created_at FROM users WHERE LOWER(username) = LOWER(?);`

	var u models.User
	var createdAtStr string

	err := db.QueryRow(query, username).Scan(&u.ID, &u.Username, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, nil // Profile doesn't exist yet
	}
	if err != nil {
		return nil, fmt.Errorf("user identity profile resolution failure: %w", err)
	}
	return &u, nil
}

// UpsertWatchProgress updates a user's running checkpoint progress ledger for a given media asset.
// It will only advance the current episode counter if the incoming episode number is greater
// than the recorded value, protecting against out-of-order stream processing.
func UpsertWatchProgress(db *sql.DB, userID int64, mediaID int64, episodeNum int, sentiment int) error {
	query := `
		INSERT INTO watch_progress (user_id, media_id, current_episode_progress, last_watched_at, sentiment)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(user_id, media_id) DO UPDATE SET
			current_episode_progress = MAX(watch_progress.current_episode_progress, excluded.current_episode_progress),
			sentiment = excluded.sentiment,
			last_watched_at = CURRENT_TIMESTAMP;
	`

	_, err := db.Exec(query, userID, mediaID, episodeNum, sentiment)
	if err != nil {
		return fmt.Errorf("failed to upsert watch progress tracker state: %w", err)
	}
	return nil
}

// SeedDefaultUsers checks if the users table is empty and injects initial profiles
// defined within the external configuration parameters.
func SeedDefaultUsers(db *sql.DB, userList []string) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users;").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check existing user count: %w", err)
	}

	// If users already exist, skip seeding to prevent overwriting active state
	if count > 0 {
		return nil
	}

	if len(userList) == 0 {
		slog.Warn("No bootstrap users defined in configuration. Skipping seeding step.")
		return nil
	}

	slog.Info("Base user profiles not detected. Seeding default suite accounts...")

	query := `INSERT INTO users (username, created_at) VALUES (?, CURRENT_TIMESTAMP);`

	for _, username := range userList {
		if _, err := db.Exec(query, username); err != nil {
			return fmt.Errorf("failed to seed user profile [%s]: %w", username, err)
		}
		slog.Info("User profile successfully bootstrapped", "username", username)
	}

	return nil
}
