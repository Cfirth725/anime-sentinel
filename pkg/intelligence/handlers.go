// Package intelligence coordinates analytical evaluation, tracking profile parsing,
// and mathematical taste anchor processing for active viewer accounts.
package intelligence

import (
	"encoding/json"
	"fmt"
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

// HandleGetSharedRecommendations cross-references two user profiles via query parameters,
// computes their mutual compatibility index, and streams filtered external suggestions.
// It mandates the presence of 'user_a' and 'user_b' parameters, returning a 400 Bad Request if missing.
func (ie *IntelligenceEngine) HandleGetSharedRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userAStr := r.URL.Query().Get("user_a")
	userBStr := r.URL.Query().Get("user_b")

	if userAStr == "" || userBStr == "" {
		http.Error(w, "Missing required query parameters 'user_a' and 'user_b'", http.StatusBadRequest)
		return
	}

	userAID, errA := strconv.ParseInt(userAStr, 10, 64)
	userBID, errB := strconv.ParseInt(userBStr, 10, 64)
	if errA != nil || errB != nil {
		http.Error(w, "Invalid user ID formatting. Must be integers.", http.StatusBadRequest)
		return
	}

	affinity, recommendations, err := ie.CalculateSharedRecommendations(userAID, userBID)
	if err != nil {
		slog.Error("Failed to compute joint-viewing metrics", "user_a", userAID, "user_b", userBID, "error", err)
		http.Error(w, "Internal calculation error aggregating external recommendations", http.StatusInternalServerError)
		return
	}

	responsePayload := map[string]interface{}{
		"compatibility_affinity": fmt.Sprintf("%.2f%%", affinity),
		"recommendations_count":  len(recommendations),
		"recommendations":        recommendations,
	}

	if len(recommendations) == 0 && affinity > 0 {
		responsePayload["notice"] = "Mutual taste profiles calculated successfully, but upstream metadata directory returned temporary server errors. Please retry shortly."
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(responsePayload); err != nil {
		slog.Error("Failed to encode joint recommendation JSON response", "error", err)
	}
}
