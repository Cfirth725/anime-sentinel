package parser

import (
	"regexp"
	"strconv"
	"strings"
)

// NormalizedMedia represents the parsed result of a raw watch log entry.
type NormalizedMedia struct {
	BaseTitle  string
	EpisodeNum float64
	IsMovie    bool
}

// Generalized expression to catch trailing season/arc/part/cour modifiers, including dashed or parenthetical iterations
var seasonRegex = regexp.MustCompile(`(?i)\s*(?::\s*|\s+|-|\()?(?:season\s*\d+|part\s*\d+|cour\s*\d+|arc\s*\d+|specials?|\d+(?:st|nd|rd|th)\s*season)(?:\s*\))?`)

// Captures optional decimal places (e.g., "Episode 13.5" or "Ep 05")
var episodeRegex = regexp.MustCompile(`(?i)(?:\s+episode\s+|\s+ep\s+)(\d+(?:\.\d+)?)`)

// NormalizeWatchEntry parses raw streaming strings into clean catalog metadata
func NormalizeWatchEntry(rawTitle string) NormalizedMedia {
	cleaned := rawTitle

	// 1. Extract episode number if it exists before stripping general text chunks
	episodeNum := 1.0
	isMovie := true

	matches := episodeRegex.FindStringSubmatch(cleaned)
	if len(matches) > 1 {
		if num, err := strconv.ParseFloat(matches[1], 64); err == nil {
			episodeNum = num
			isMovie = false
		}
		// Clean the episode marker text out of the working title string
		cleaned = episodeRegex.ReplaceAllString(cleaned, "")
	}

	// 2. Strip out season subtitles/modifiers to extract the core base series title
	cleaned = seasonRegex.ReplaceAllString(cleaned, "")

	// 3. Perform final text sanitation on remaining whitespace or dangling punctuation symbols
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimSuffix(cleaned, ":")
	cleaned = strings.TrimSuffix(cleaned, "-")
	cleaned = strings.TrimSpace(cleaned) // Final catch for trailing space after trim

	return NormalizedMedia{
		BaseTitle:  cleaned,
		EpisodeNum: episodeNum,
		IsMovie:    isMovie,
	}
}
