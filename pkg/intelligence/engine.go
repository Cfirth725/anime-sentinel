package intelligence

import (
	"database/sql"
	"log/slog"

	"github.com/Cfirth725/anime-sentinel/pkg/database"
	"github.com/Cfirth725/anime-sentinel/pkg/models"
)

// IntelligenceEngine orchestrates taste profiling metrics and joint-viewing analytics.
type IntelligenceEngine struct {
	db *sql.DB
}

// NewIntelligenceEngine instantiates the analytical computation component.
func NewIntelligenceEngine(db *sql.DB) *IntelligenceEngine {
	return &IntelligenceEngine{db: db}
}

// ExtractTasteAnchors filters a user's engagement profiles down to their core mathematical preferences.
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

	slog.Info("Taste profiling complete", "user_id", userID, "total_tracked", len(allProfiles), "taste_anchors", len(anchors))
	return anchors, nil
}
