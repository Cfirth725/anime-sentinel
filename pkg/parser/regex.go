package parser

import (
	"regexp"
	"strconv"
	"strings"
)

// NormalizedMedia represents the parsed result of a raw watch log entry
type NormalizedMedia struct {
	BaseTitle  string
	EpisodeNum int
	IsMovie    bool
}

// Expressions to strip season modifiers (e.g., "Stone Wars", "Season 2", "Part 1")
var seasonRegex = regexp.MustCompile(`(?i)(:\s*stone wars|:\s*season\s*\d+|:\s*part\s*\d+|:\s*cour\s*\d+)`)

// Patterns to isolate episode markers (e.g., "Episode 05", "Ep 12")
var episodeRegex = regexp.MustCompile(`(?i)(?:\s+episode\s+|\s+ep\s+)(\d+)`)

// NormalizeWatchEntry parses raw streaming strings into clean catalog metadata
func NormalizeWatchEntry(rawTitle string) NormalizedMedia {
	cleaned := rawTitle

	// 1. Strip out season subtitles/modifiers to extract the core base series title
	cleaned = seasonRegex.ReplaceAllString(cleaned, "")

	// 2. Identify layout format and extract episode number if it exists
	episodeNum := 1
	isMovie := true

	matches := episodeRegex.FindStringSubmatch(cleaned)
	if len(matches) > 1 {
		if num, err := strconv.Atoi(matches[1]); err == nil {
			episodeNum = num
			isMovie = false
		}
		// Clean the episode marker text out of the title string entirely
		cleaned = episodeRegex.ReplaceAllString(cleaned, "")
	}

	// 3. Perform final text sanitation on remaining whitespace or trailing colons
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimSuffix(cleaned, ":")

	return NormalizedMedia{
		BaseTitle:  cleaned,
		EpisodeNum: episodeNum,
		IsMovie:    isMovie,
	}
}
