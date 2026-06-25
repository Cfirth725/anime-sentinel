// Package intelligence coordinates analytical evaluation, tracking profile parsing,
// and mathematical taste anchor processing for active viewer accounts.
package intelligence

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/Cfirth725/anime-sentinel/pkg/database"
	"github.com/Cfirth725/anime-sentinel/pkg/metadata"
	"github.com/Cfirth725/anime-sentinel/pkg/models"
)

// IntelligenceEngine orchestrates taste profiling metrics and engagement analytics.
type IntelligenceEngine struct {
	db       *sql.DB
	alClient *metadata.AniListClient
}

// NewIntelligenceEngine instantiates the analytical computation components.
func NewIntelligenceEngine(db *sql.DB, alClient *metadata.AniListClient) *IntelligenceEngine {
	return &IntelligenceEngine{
		db:       db,
		alClient: alClient,
	}
}

// ExtractTasteAnchors filters a user's engagement profiles down to their core mathematical preferences.
// It returns a slice of user engagement rows flagged as taste anchors based on high consumption benchmarks.
func (ie *IntelligenceEngine) ExtractTasteAnchors(userID int64) ([]models.UserEngagement, error) {
	allProfiles, err := database.GetUserEngagementProfiles(ie.db, userID)
	if err != nil {
		return nil, err
	}

	var anchors []models.UserEngagement
	for _, prof := range allProfiles {
		if prof.IsTasteAnchor {
			anchors = append(anchors, prof)
		}
	}

	slog.Info("Taste profiling complete",
		"user_id", userID,
		"total_tracked", len(allProfiles),
		"taste_anchors", len(anchors),
	)
	return anchors, nil
}

// CalculateSharedRecommendations analyzes the historical overlap between two profiles,
// computes a mutual affinity rating, pulls external suggestions from AniList, and filters out already-seen content.
func (ie *IntelligenceEngine) CalculateSharedRecommendations(userAID, userBID int64) (float64, []models.RecommendationNode, error) {
	// 1. Fetch local engagement profiles for both users
	profilesA, err := database.GetUserEngagementProfiles(ie.db, userAID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to retrieve profiles for user A: %w", err)
	}
	profilesB, err := database.GetUserEngagementProfiles(ie.db, userBID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to retrieve profiles for user B: %w", err)
	}

	// Maps for quick cross-referencing using the internal MediaID keys
	seenOrTracked := make(map[int64]bool)
	anchorsA := make(map[int64]bool)

	for _, p := range profilesA {
		seenOrTracked[p.MediaID] = true
		if p.IsTasteAnchor {
			anchorsA[p.MediaID] = true
		}
	}

	var mutualAnchorCount int
	var sharedSeeds []models.UserEngagement

	for _, p := range profilesB {
		seenOrTracked[p.MediaID] = true
		if p.IsTasteAnchor {
			// If both users flagged this title as an 80%+ taste anchor, it's a mutual match!
			if anchorsA[p.MediaID] {
				mutualAnchorCount++
				sharedSeeds = append(sharedSeeds, p)
			}
		}
	}

	// Compute overall Compatibility Affinity Metric
	totalUniqueTitles := len(seenOrTracked)
	if totalUniqueTitles == 0 {
		return 0, nil, nil
	}
	affinityScore := (float64(mutualAnchorCount) / float64(totalUniqueTitles)) * 100.0

	slog.Info("Calculated mutual profile metrics",
		"user_a", userAID,
		"user_b", userBID,
		"mutual_anchors", mutualAnchorCount,
		"compatibility_affinity", fmt.Sprintf("%.2f%%", affinityScore),
	)

	// If there's no common ground yet, return early
	if len(sharedSeeds) == 0 {
		return affinityScore, []models.RecommendationNode{}, nil
	}

	// 2. Pull external recommendations via AniList using shared seeds
	var rawRecommendations []models.RecommendationNode

	lookupLimit := len(sharedSeeds)
	if lookupLimit > 3 {
		lookupLimit = 3
	}

	for i := 0; i < lookupLimit; i++ {
		seed := sharedSeeds[i]

		// Use your existing GetMediaByTitle query to resolve the string ExternalID from the cache layer
		cachedMedia, err := database.GetMediaByTitle(ie.db, seed.BaseTitle)
		if err != nil || cachedMedia == nil {
			slog.Warn("Skipping recommendation seed: title missing from local catalog cache", "title", seed.BaseTitle)
			continue
		}

		slog.Debug("Querying external relational recommendation edges", "seed_title", cachedMedia.TitleRomaji)

		recs, err := ie.alClient.FetchRecommendationsForSeries(cachedMedia.ExternalID)
		if err != nil {
			slog.Warn("Skipping recommendation seed node due to upstream error", "external_id", cachedMedia.ExternalID, "error", err)
			continue
		}
		if recs != nil {
			rawRecommendations = append(rawRecommendations, recs.Nodes...)
		}
	}

	// 3. Deduplicate and filter out shows either user has already watched locally
	var finalRecommendations []models.RecommendationNode
	uniqueRecCheck := make(map[int]bool)

	for _, node := range rawRecommendations {
		if node.MediaRecommendation == nil {
			continue
		}

		recID := node.MediaRecommendation.ID

		// Determine the primary display title (Prefer English, fallback to Romaji)
		displayTitle := node.MediaRecommendation.Title.English
		if displayTitle == "" {
			displayTitle = node.MediaRecommendation.Title.Romaji
		}

		// Cross-reference the recommended target by checking if it already exists in your local cache.
		// We pass the display title name to see if it maps to an already seen MediaID.
		localMedia, err := database.GetMediaByTitle(ie.db, displayTitle)
		if err == nil && localMedia != nil {
			if seenOrTracked[localMedia.ID] {
				continue
			}
		}

		// Also check the Romaji title against the local cache just in case your history used the Romaji name
		if node.MediaRecommendation.Title.Romaji != "" && displayTitle != node.MediaRecommendation.Title.Romaji {
			romajiMedia, err := database.GetMediaByTitle(ie.db, node.MediaRecommendation.Title.Romaji)
			if err == nil && romajiMedia != nil {
				if seenOrTracked[romajiMedia.ID] {
					continue
				}
			}
		}

		if uniqueRecCheck[recID] {
			continue
		}

		uniqueRecCheck[recID] = true

		// If an English title exists, copy it over to the Romaji field or ensure it's prioritized
		// in the payload structure before returning it to the HTTP response layer.
		finalRecommendations = append(finalRecommendations, node)
	}
	return affinityScore, finalRecommendations, nil
}
