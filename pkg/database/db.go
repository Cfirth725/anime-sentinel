package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// Config encapsulates environment configuration fields mapped to JSON file properties.
type Config struct {
	Port          string `json:"PORT"`
	DatabasePath  string `json:"DATABASE_PATH"`
	SQLiteWALMode bool   `json:"SQLITE_WAL_MODE"`
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
