package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	SeriesTitle   string `json:"series_title"`
	SeasonTitle   string `json:"season_title"`
	EpisodeNumber int    `json:"episode_number"`
}

// Sentinel Ingest Target Format
type IngestPayload struct {
	Username  string `json:"username"`
	RawTitle  string `json:"raw_title"`
	Sentiment int    `json:"sentiment"`
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

	var finalPayload []IngestPayload

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
			// Forces messy franchises to collapse to their searchable core names
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

			// 1. ULTIMATE FALLBACK: If the season title is empty, or if it doesn't
			// include the core series name, safely combine them!
			if displayTitle == "" {
				displayTitle = meta.SeriesTitle
			} else if !strings.Contains(strings.ToLower(displayTitle), strings.ToLower(meta.SeriesTitle)) {
				// Turning "Black Clover" and "Season 1 Part 4" into "Black Clover: Season 1 Part 4"
				displayTitle = fmt.Sprintf("%s: %s", meta.SeriesTitle, displayTitle)
			}

			// 2. MASSIVE TITAN CLEANER: Counts from 100 down to 1
			// Completely future-proofed for long-running series like One Piece!
			for i := 100; i >= 1; i-- {
				numStr := strconv.Itoa(i)

				displayTitle = strings.ReplaceAll(displayTitle, " Part "+numStr, "")
				displayTitle = strings.ReplaceAll(displayTitle, " Season "+numStr, "")
			}

			// Clean up any double colons left over from the truncation
			displayTitle = strings.ReplaceAll(displayTitle, ": :", ":")
			displayTitle = strings.ReplaceAll(displayTitle, "::", ":")
			displayTitle = strings.TrimSuffix(displayTitle, ":")

			title := fmt.Sprintf("%s: Episode %d", displayTitle, meta.EpisodeNumber)

			finalPayload = append(finalPayload, IngestPayload{
				Username:  "Carolyn",
				RawTitle:  title,
				Sentiment: 1,
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

	fmt.Printf("Migration successful! Converted %d history logs.\n", len(finalPayload))
}
