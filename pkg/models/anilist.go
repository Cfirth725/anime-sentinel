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

// AniListMedia captures the specific metadata fields defined in the Phase 4 catalog schema.
type AniListMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
	} `json:"title"`
	Format   string `json:"format"`
	Status   string `json:"status"`
	Episodes *int   `json:"episodes"` // Pointer handles cases where episode counts are null (releasing)
}
