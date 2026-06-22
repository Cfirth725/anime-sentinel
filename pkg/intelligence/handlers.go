package intelligence

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// HandleGetTasteAnchors returns the computed mathematical taste anchors for a given user.
func (ie *IntelligenceEngine) HandleGetTasteAnchors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// For now, default to User ID 1 (Carolyn) if no ID parameter is passed
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
	json.NewEncoder(w).Encode(anchors)
}
