// Package intelligence coordinates analytical evaluation, tracking profile parsing,
// and mathematical taste anchor processing for active viewer accounts.
package intelligence

import (
	"database/sql"
	"log/slog"

	"github.com/Cfirth725/anime-sentinel/pkg/database"
	"github.com/Cfirth725/anime-sentinel/pkg/models"
)

// IntelligenceEngine orchestrates taste profiling metrics and engagement analytics.
type IntelligenceEngine struct {
	db *sql.DB
}

// NewIntelligenceEngine instantiates the analytical computation components.
func NewIntelligenceEngine(db *sql.DB) *IntelligenceEngine {
	return &IntelligenceEngine{db: db}
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
