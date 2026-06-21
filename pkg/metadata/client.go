package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Cfirth725/anime-sentinel/pkg/models"
)

// AniListClient coordinates throttled outbound communication with the AniList GraphQL API.
type AniListClient struct {
	httpClient *http.Client
	ticker     *time.Ticker
	endpoint   string
}

// NewAniListClient initializes a thread-safe synchronization client.
// A 700ms ticker interval guarantees a maximum cadence of ~85 requests per minute,
// keeping the pipeline safely below AniList's hard threshold of 90 requests/min.
func NewAniListClient() *AniListClient {
	return &AniListClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		ticker:   time.NewTicker(700 * time.Millisecond),
		endpoint: "https://graphql.anilist.co",
	}
}

// FetchSeriesMetadata blocks execution against the internal ticker before dispatching
// a GraphQL request to safely search and resolve clean media metadata.
// It incorporates an exponential backoff retry mechanism to survive upstream 429 rate limits.
func (c *AniListClient) FetchSeriesMetadata(cleanTitle string) (*models.AniListMedia, error) {
	// STOP THE THROTTLE: Park the routine until the shared ticker drops a clock token.
	<-c.ticker.C

	// Hardcoded GraphQL query string optimized for core title and format matching
	query := `
		query ($search: String) {
			Media (search: $search, type: ANIME) {
				id
				title {
					romaji
					english
				}
				format
				status
				episodes
			}
		}
	`

	// Build the network request frame
	requestPayload := models.AniListRequest{
		Query: query,
		Variables: map[string]interface{}{
			"search": cleanTitle,
		},
	}

	bodyBytes, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL payload: %w", err)
	}

	maxRetries := 3
	backoffDuration := 4 * time.Second
	var resp *http.Response

	// Execute the HTTP transport retry loop
	for i := 0; i < maxRetries; i++ {
		slog.Debug("Rate limit token acquired. Dispatching upstream metadata query", "search_title", cleanTitle, "attempt", i+1)

		// Create a fresh request body reader stream for each retry attempt
		req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewBuffer(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to construct outbound HTTP request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("network execution failed against AniList gateway: %w", err)
		}

		// Handle 429 Rate Limits with Exponential Backoff
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close() // Close the body immediately since we are retrying

			slog.Warn("🛑 Upstream 429 rate limit breached. Initiating cool-down backoff period...",
				"title", cleanTitle,
				"delay_seconds", backoffDuration.Seconds(),
				"attempt", i+1,
			)

			time.Sleep(backoffDuration)
			backoffDuration *= 2 // Double the pause duration exponentially for the next pass
			continue
		}

		// Break out of the retry loop if we get any other status code (200, 404, 500, etc.)
		break
	}

	defer resp.Body.Close()

	// Intercept API errors early
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream API returned non-200 status code: %d", resp.StatusCode)
	}

	// Parse the response
	var aniListResp models.AniListResponse
	if err := json.NewDecoder(resp.Body).Decode(&aniListResp); err != nil {
		return nil, fmt.Errorf("failed to decode upstream JSON response structure: %w", err)
	}

	// Return the nested media struct pointer (returns nil if no matches were located)
	return aniListResp.Data.Media, nil
}

// Close teardown internal ticker instances gracefully to prevent system routine leaks.
func (c *AniListClient) Close() {
	c.ticker.Stop()
}
