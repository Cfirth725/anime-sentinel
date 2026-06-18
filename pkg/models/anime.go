package models

import "time"

// IngestPayload mirrors high-volume historical tracking data arriving at the API gateway.
// Struct tags facilitate rapid JSON streaming unmarshalling from network requests.
type IngestPayload struct {
	Username   string `json:"username"`
	RawTitle   string `json:"raw_title"`
	Sentiment  int    `json:"sentiment"` // Enforces underlying database check constraint: [-1, 0, 1]
	ExternalID string `json:"external_id,omitempty"`
}

// User represents a local account identity mapping directly to the relational database users table.
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

// MediaCatalog represents the local autonomous cache layer for third-party metadata.
// Fields align with checked relational data constraints for formats and release status.
type MediaCatalog struct {
	ID                 int64     `json:"id"`
	ExternalID         string    `json:"external_id"` // Matches upstream tracking IDs (e.g., AniList API)
	TitleRomaji        string    `json:"title_romaji"`
	TitleEnglish       string    `json:"title_english,omitempty"`
	Format             string    `json:"format"` // Restricts to allowed enum subsets: TV, MOVIE, OVA, SPECIAL
	Status             string    `json:"status"` // Restricts to tracking subsets: FINISHED, RELEASING, NOT_YET_RELEASED
	TotalEpisodesCount int       `json:"total_episodes_count"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// WatchProgress records isolated user sync checkpoints mapping to progress ledger tables.
// Incorporates the synchronized state tracking metrics flag to reflect user profile sentiment.
type WatchProgress struct {
	UserID                 int64     `json:"user_id"`
	MediaID                int64     `json:"media_id"`
	CurrentEpisodeProgress int       `json:"current_episode_progress"`
	LastWatchedAt          time.Time `json:"last_watched_at"`
	Sentiment              int       `json:"sentiment"` // Synchronized state tracking flag: -1 = Bad, 0 = Neutral, 1 = Good
}
