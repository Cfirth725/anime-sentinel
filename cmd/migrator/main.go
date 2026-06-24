package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Cfirth725/anime-sentinel/pkg/models"
)

// Crunchyroll Schema Mappings
type CrHistory struct {
	Data []CrItem `json:"data"`
}

type CrItem struct {
	Panel CrPanel `json:"panel"`
}

type CrPanel struct {
	EpisodeMetadata CrMetadata `json:"episode_metadata"`
}

type CrMetadata struct {
	SeriesTitle   string  `json:"series_title"`
	SeasonTitle   string  `json:"season_title"`
	EpisodeNumber float64 `json:"episode_number"`
}

func main() {
	// Read your raw pasted Crunchyroll file
	rawData, err := os.ReadFile("raw_crunchyroll.json")
	if err != nil {
		fmt.Printf("Error reading raw file: %v\n", err)
		return
	}

	// Read as a slice of history page chunks
	var pages []CrHistory
	if err := json.Unmarshal(rawData, &pages); err != nil {
		fmt.Printf("Error parsing multi-page Crunchyroll JSON: %v\n", err)
		return
	}

	var finalPayload []models.IngestPayload

	// Loop over each page chunk, then loop over the data rows inside them
	for _, page := range pages {
		for _, item := range page.Data {
			meta := item.Panel.EpisodeMetadata
			if meta.SeriesTitle == "" {
				continue
			}

			// Scrub streaming dub text out-of-the-gate
			meta.SeriesTitle = strings.ReplaceAll(meta.SeriesTitle, " (English Dub)", "")

			// ----- MASTER CORRECTION LAYER -----
			if strings.Contains(strings.ToLower(meta.SeriesTitle), "re:zero") {
				if strings.Contains(meta.SeasonTitle, "E-EX") {
					meta.SeriesTitle = "Re:ZERO -Starting Life in Another World-: OVAs"
					meta.SeasonTitle = ""
				} else {
					meta.SeriesTitle = "Re:ZERO -Starting Life in Another World-"
					meta.SeasonTitle = ""
				}
			} else if strings.Contains(strings.ToLower(meta.SeriesTitle), "demon slayer") {
				meta.SeriesTitle = "Demon Slayer: Kimetsu no Yaiba"
				meta.SeasonTitle = ""
			} else if strings.Contains(strings.ToLower(meta.SeriesTitle), "frieren") {
				meta.SeriesTitle = "Frieren"
				meta.SeasonTitle = ""
			} else if strings.Contains(strings.ToLower(meta.SeriesTitle), "attack on titan") {
				meta.SeriesTitle = "Shingeki no Kyojin"
				meta.SeasonTitle = ""
			}

			displayTitle := meta.SeasonTitle

			// 1. ULTIMATE FALLBACK
			if displayTitle == "" {
				displayTitle = meta.SeriesTitle
			} else if !strings.Contains(strings.ToLower(displayTitle), strings.ToLower(meta.SeriesTitle)) {
				displayTitle = fmt.Sprintf("%s: %s", meta.SeriesTitle, displayTitle)
			}

			// 2. MASSIVE TITAN CLEANER
			for i := 100; i >= 1; i-- {
				numStr := strconv.Itoa(i)
				displayTitle = strings.ReplaceAll(displayTitle, " Part "+numStr, "")
				displayTitle = strings.ReplaceAll(displayTitle, " Season "+numStr, "")
			}

			// Clean up loose punctuation
			displayTitle = strings.ReplaceAll(displayTitle, ": :", ":")
			displayTitle = strings.ReplaceAll(displayTitle, "::", ":")
			displayTitle = strings.TrimSuffix(displayTitle, ":")

			// Populate the rich target struct parameters exactly as engine.go expects them
			finalPayload = append(finalPayload, models.IngestPayload{
				Username:      "Carolyn",
				Sentiment:     1,
				SeriesID:      "GENERIC_MIGRATION",
				SeriesTitle:   displayTitle,
				SeasonNumber:  1,
				EpisodeNumber: meta.EpisodeNumber,
				EpisodeTitle:  "Imported History Entry",
				EpisodeID:     "GENERIC_EPISODE",
				WatchedAt:     time.Now(),
				FullyWatched:  true,
			})
		}
	}

	// Marshal into a beautifully formatted output file
	outBytes, err := json.MarshalIndent(finalPayload, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling output data: %v\n", err)
		return
	}

	err = os.WriteFile("crunchyroll_import.json", outBytes, 0644)
	if err != nil {
		fmt.Printf("Error writing clean file: %v\n", err)
		return
	}

	fmt.Printf("Migration successful! Converted %d history logs into rich IngestPayload schema.\n", len(finalPayload))
}
