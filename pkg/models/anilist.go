// Package models encapsulates the core domain types, bulk payload envelopes,
// API request/response frames, and database transfer objects used throughout the system.
package models

// ====================================================================
//             -- UPSTREAM ANILIST GRAPHQL CONTRACTS --
// ====================================================================

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

// ====================================================================
//             -- UPSTREAM ANILIST CATALOG DATA SHAPES --
// ====================================================================

// AniListMedia captures the specific metadata fields mapped from the external third-party registry.
type AniListMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
	} `json:"title"`
	Format          string                    `json:"format"`
	Status          string                    `json:"status"`
	Episodes        *int                      `json:"episodes"` // Pointer safely absorbs JSON null values for ongoing series.
	Recommendations *RecommendationConnection `json:"recommendations"`
}

// ====================================================================
//             -- UPSTREAM ANILIST RELATIONAL EDGES --
// ====================================================================

// RecommendationConnection encapsulates the edge framing for nested relational recommendations.
type RecommendationConnection struct {
	Nodes []RecommendationNode `json:"nodes"`
}

// RecommendationNode extracts the concrete media payload of a recommended asset from the API.
type RecommendationNode struct {
	Rating              int `json:"rating"` // The crowdsourced community approval weight
	MediaRecommendation *struct {
		ID    int `json:"id"`
		Title struct {
			Romaji  string `json:"romaji"`
			English string `json:"english"`
		} `json:"title"`
		Format   string `json:"format"`
		Status   string `json:"status"`
		Episodes *int   `json:"episodes"`
	} `json:"mediaRecommendation"`
}
