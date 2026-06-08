package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// Config holds the local environment configuration parameters
type Config struct {
	Port          string `json:"PORT"`
	DatabasePath  string `json:"DATABASE_PATH"`
	SQLiteWALMode bool   `json:"SQLITE_WAL_MODE"`
}

// LoadConfig reads the un-tracked local json parameter file
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

// InitDB initializes the SQLite connection and ingests the external schema file
func InitDB(dbPath string, useWAL bool, schemaFilePath string) (*sql.DB, error) {
	// 1. Establish file descriptor link
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// 2. Configure high-concurrency multi-process WAL settings
	if useWAL {
		if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
		}
		slog.Info("🔒 SQLite engine initialized with Write-Ahead Logging (WAL)")
	}

	// 3. Read the external schema.sql file
	schemaBytes, err := os.ReadFile(schemaFilePath)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to read schema file at %s: %w", schemaFilePath, err)
	}

	// 4. Execute schema payload
	if _, err := db.Exec(string(schemaBytes)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute external schema sql: %w", err)
	}

	slog.Info("📊 Relational schema and indexing partitions verified from schema.sql")
	return db, nil
}
