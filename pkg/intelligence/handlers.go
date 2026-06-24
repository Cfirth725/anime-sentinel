// Package intelligence coordinates analytical evaluation, tracking profile parsing,
// and mathematical taste anchor processing for active viewer accounts.
package intelligence

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// HandleGetTasteAnchors extracts and returns computed mathematical taste anchors for a given user.
// It maps the resultant slice to JSON, defaulting to user ID 1 if no identifier is explicitly passed.
func (ie *IntelligenceEngine) HandleGetTasteAnchors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Default to User ID 1 if no ID parameter is passed
	userIDStr := r.URL.Query().Get("user_id")
	userID := int64(1)
	if userIDStr != "" {
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err == nil {
			userID = id
		}
	}

	anchors, err := ie.ExtractTasteAnchors(userID)
	if err != nil {
		slog.Error("Failed to extract taste anchors", "user_id", userID, "error", err)
		http.Error(w, "Internal server error calculating analytics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(anchors); err != nil {
		slog.Error("Failed to encode taste anchors payload response", "user_id", userID, "error", err)
	}
}
