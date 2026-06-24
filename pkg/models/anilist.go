// Package models encapsulates the core domain types, bulk payload envelopes,
// API request/response frames, and database transfer objects used throughout the system.
package models

// AniListRequest encapsulates the standard GraphQL POST payload footprint.
type AniListRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// AniListResponse represents the nested JSON payload returned by the AniList API gateway.
type AniListResponse struct {
	Data struct {
		Media *AniListMedia `json:"Media"`
	} `json:"data"`
}

// AniListMedia captures the specific metadata fields mapped from the external third-party registry.
type AniListMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
	} `json:"title"`
	Format   string `json:"format"`
	Status   string `json:"status"`
	Episodes *int   `json:"episodes"` // Pointer safely absorbs JSON null values for ongoing series.
}
