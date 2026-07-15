// Package metadata coordinates outbound communications, rate limiting controls,
// and retry state engines targeting third-party streaming directories.
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

// ====================================================================
//                -- CLIENT CONFIGURATION & INTAKE --
// ====================================================================

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

// AcquireToken blocks execution against the internal shared ticker.
// This allows background workers to pace their network access gates cooperatively.
func (c *AniListClient) AcquireToken() {
	<-c.ticker.C
}

// Close teardown internal ticker instances gracefully to prevent system routine leaks.
func (c *AniListClient) Close() {
	c.ticker.Stop()
}

// ====================================================================
//             -- OUTBOUND EXTERNAL API SYNCHRONIZERS --
// ====================================================================

// FetchSeriesMetadata dispatches a GraphQL request to safely search and resolve clean media metadata.
// Callers should invoke AcquireToken() before calling this method to enforce target rate limits.
// It incorporates an exponential backoff retry mechanism to handle upstream 429 rate limits gracefully.
func (c *AniListClient) FetchSeriesMetadata(cleanTitle string) (*models.AniListMedia, error) {
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

	for i := 0; i < maxRetries; i++ {
		slog.Debug("[REALTIME] Rate limit token acquired. Dispatching upstream metadata query", "search_title", cleanTitle, "attempt", i+1)

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

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()

			slog.Warn("[REALTIME] Upstream 429 rate limit breached. Initiating cool-down backoff period...",
				"title", cleanTitle,
				"delay_seconds", backoffDuration.Seconds(),
				"attempt", i+1,
			)

			time.Sleep(backoffDuration)
			backoffDuration *= 2
			continue
		}

		break
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream API returned non-200 status code: %d", resp.StatusCode)
	}

	var aniListResp models.AniListResponse
	if err := json.NewDecoder(resp.Body).Decode(&aniListResp); err != nil {
		return nil, fmt.Errorf("failed to decode upstream JSON response structure: %w", err)
	}

	return aniListResp.Data.Media, nil
}

// FetchRecommendationsForSeries queries AniList for highly-rated community recommendations
// tied directly to a specific external series ID.
func (c *AniListClient) FetchRecommendationsForSeries(externalID string) (*models.RecommendationConnection, error) {
	query := `
		query ($id: Int) {
			Media (id: $id, type: ANIME) {
				recommendations (sort: RATING_DESC, perPage: 10) {
					nodes {
						rating
						mediaRecommendation {
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
				}
			}
		}
	`

	requestPayload := models.AniListRequest{
		Query: query,
		Variables: map[string]interface{}{
			"id": externalID,
		},
	}

	bodyBytes, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal recommendation query: %w", err)
	}

	c.AcquireToken()

	slog.Debug("[REALTIME] Rate limit token acquired. Dispatching upstream relational recommendations query", "external_id", externalID)

	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to construct request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network execution failed against AniList recommendations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream API returned non-200 status code: %d", resp.StatusCode)
	}

	var aniListResp models.AniListResponse
	if err := json.NewDecoder(resp.Body).Decode(&aniListResp); err != nil {
		return nil, fmt.Errorf("failed to decode recommendation response: %w", err)
	}

	if aniListResp.Data.Media == nil {
		return nil, nil
	}

	return aniListResp.Data.Media.Recommendations, nil
}
