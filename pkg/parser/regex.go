// Package parser provides text normalization utilities and pattern matching engines
// to sanitize raw media history streams before ingestion.
package parser

import (
	"regexp"
	"strconv"
	"strings"
)

// NormalizedMedia represents the structural result of a parsed raw watch log entry.
type NormalizedMedia struct {
	BaseTitle  string  // The core series name stripped of metadata chunks.
	EpisodeNum float64 // The extracted episode sequence number or fallback decimal.
	IsMovie    bool    // True if no explicit serial episode markers were detected.
}

var (
	// seasonRegex is a generalized expression to catch trailing season/arc/part/cour modifiers,
	// including dashed, underscored, or parenthetical iterations.
	seasonRegex = regexp.MustCompile(`(?i)\s*(?::\s*|\s+|-|\()?(?:season\s*\d+|part\s*\d+|cour\s*\d+|arc\s*\d+|specials?|\d+(?:st|nd|rd|th)\s*season)(?:\s*\))?`)

	// episodeRegex captures serial episode progressions including optional decimal places.
	episodeRegex = regexp.MustCompile(`(?i)(?:\s+episode\s+|\s+ep\s+)(\d+(?:\.\d+)?)`)
)

// NormalizeWatchEntry parses raw streaming history strings into a clean, uniform metadata layout.
// It strips out chronological markers and calculates contextual movie/series flags.
func NormalizeWatchEntry(rawTitle string) NormalizedMedia {
	cleaned := rawTitle
	episodeNum := 1.0
	isMovie := true

	// Extract the episode identifier pattern to map progress sequence and evaluate tracking format flags.
	matches := episodeRegex.FindStringSubmatch(cleaned)
	if len(matches) > 1 {
		if num, err := strconv.ParseFloat(matches[1], 64); err == nil {
			episodeNum = num
			isMovie = false
		}
		cleaned = episodeRegex.ReplaceAllString(cleaned, "")
	}

	// Strip out active season subtitles and metadata modifiers to isolate the underlying base series name.
	cleaned = seasonRegex.ReplaceAllString(cleaned, "")

	// Perform final text sanitation to strip leading/trailing whitespace and dangling structural punctuation symbols.
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.TrimFunc(cleaned, func(r rune) bool {
		return r == ':' || r == '-' || r == ',' || r == '.'
	})
	cleaned = strings.TrimSpace(cleaned)

	return NormalizedMedia{
		BaseTitle:  cleaned,
		EpisodeNum: episodeNum,
		IsMovie:    isMovie,
	}
}
