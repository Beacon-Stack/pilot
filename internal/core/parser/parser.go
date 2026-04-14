// Package parser extracts structured metadata (show name, season, episode
// numbers, quality, release group, etc.) from release filenames and titles.
//
// This is a minimal implementation that covers the common S01E05 / S01E05E06
// patterns required by the importer.  A richer implementation (handling daily
// episodes, anime absolute numbering, scene naming conventions) can replace
// this file without changing the public API.
package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// EpisodeInfo holds the TV-episode-specific fields parsed from a filename.
type EpisodeInfo struct {
	// Season is the parsed season number (1-indexed; 0 = specials / unknown).
	Season int

	// Episodes is the list of episode numbers found in the filename.
	// A multi-episode file like S02E03E04 will have [3, 4].
	Episodes []int

	// IsSeasonPack is true when the filename or directory name indicates an
	// entire season rather than individual episodes (e.g. "Show.S02.BluRay").
	IsSeasonPack bool
}

// ParsedRelease is the result of parsing a single filename or release title.
type ParsedRelease struct {
	// ShowTitle is the show name portion of the filename, with dots/underscores
	// replaced by spaces and trimmed.
	ShowTitle string

	// EpisodeInfo carries the season/episode position data.
	EpisodeInfo EpisodeInfo

	// Year is the 4-digit year if one was present in the title (0 = absent).
	Year int
}

// seEpisodeRe matches patterns like S01E02, S01E02E03, s1e2, etc.
// Group 1: season; groups 2…n: episode numbers (one per "E\d+" token).
var seEpisodeRe = regexp.MustCompile(`(?i)[._\s-]S(\d{1,2})((?:E\d{1,3})+)`)

// seasonPackRe matches a bare season marker with no episode number (e.g. S02 or Season.2).
var seasonPackRe = regexp.MustCompile(`(?i)(?:^|[._\s-])S(\d{1,2})(?:[._\s-]|$)`)

// episodeNumsRe extracts individual episode numbers from a token like E01E02E03.
var episodeNumsRe = regexp.MustCompile(`(?i)E(\d{1,3})`)

// yearRe matches a 4-digit year (1900–2099).
var yearRe = regexp.MustCompile(`(?:^|[._\s(-])((?:19|20)\d{2})(?:[._\s)-]|$)`)

// Parse extracts structured information from a filename or release title.
// It is intentionally lenient: fields that cannot be determined are left at
// their zero values rather than returning an error.
func Parse(filename string) ParsedRelease {
	// Strip leading path component if present.
	filename = strings.TrimSuffix(filename, "/")
	if idx := strings.LastIndexAny(filename, "/\\"); idx >= 0 {
		filename = filename[idx+1:]
	}
	// Strip the file extension.
	if dot := strings.LastIndex(filename, "."); dot > 0 {
		filename = filename[:dot]
	}

	var result ParsedRelease

	// ── Year ─────────────────────────────────────────────────────────────────
	if m := yearRe.FindStringSubmatch(filename); len(m) >= 2 {
		if y, err := strconv.Atoi(m[1]); err == nil {
			result.Year = y
		}
	}

	// ── Season + episode numbers ──────────────────────────────────────────────
	if m := seEpisodeRe.FindStringSubmatch(filename); len(m) >= 3 {
		season, _ := strconv.Atoi(m[1])
		result.EpisodeInfo.Season = season

		for _, em := range episodeNumsRe.FindAllStringSubmatch(m[2], -1) {
			n, _ := strconv.Atoi(em[1])
			result.EpisodeInfo.Episodes = append(result.EpisodeInfo.Episodes, n)
		}

		// Extract show title: everything before the SxxExx token.
		titlePart := seEpisodeRe.Split(filename, 2)[0]
		result.ShowTitle = cleanTitle(titlePart)
		return result
	}

	// ── Season pack (no episode number) ──────────────────────────────────────
	if m := seasonPackRe.FindStringSubmatch(filename); len(m) >= 2 {
		season, _ := strconv.Atoi(m[1])
		result.EpisodeInfo.Season = season
		result.EpisodeInfo.IsSeasonPack = true

		titlePart := seasonPackRe.Split(filename, 2)[0]
		result.ShowTitle = cleanTitle(titlePart)
		return result
	}

	// ── No season/episode information found ───────────────────────────────────
	result.ShowTitle = cleanTitle(filename)
	return result
}

// NormalizeTitle lowercases a title and strips all non-alphanumeric
// characters so two titles can be compared without being tripped up by
// dots, underscores, spaces, or punctuation.
//
// Example:
//
//	NormalizeTitle("Breaking.Bad") == NormalizeTitle("breaking bad") == "breakingbad"
func NormalizeTitle(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TitleMatches reports whether a release's parsed show title refers to the
// same series as seriesTitle. Comparison is strict after normalization, with
// one concession: a trailing 4-digit year on the release side is allowed
// ("Breaking Bad 2008" matches "Breaking Bad") since indexer releases
// frequently embed the premiere year.
//
// It does NOT do any fuzzy matching, substring matching, or word-set
// comparison — those let unrelated releases slip through and cause the
// exact wrong-torrent bug this is meant to prevent.
func TitleMatches(seriesTitle, releaseShowTitle string) bool {
	s := NormalizeTitle(seriesTitle)
	r := NormalizeTitle(releaseShowTitle)
	if s == "" || r == "" {
		return false
	}
	if s == r {
		return true
	}
	if strings.HasPrefix(r, s) {
		rest := r[len(s):]
		if len(rest) == 4 && rest >= "1900" && rest <= "2099" {
			return true
		}
	}
	return false
}

// cleanTitle replaces dots, underscores, and multiple spaces with a single
// space and trims whitespace from both ends.
func cleanTitle(s string) string {
	s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
	// Collapse runs of whitespace.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// String returns a human-readable representation of a ParsedRelease for
// logging and debugging.
func (p ParsedRelease) String() string {
	ei := p.EpisodeInfo
	if len(ei.Episodes) > 0 {
		eps := make([]string, len(ei.Episodes))
		for i, e := range ei.Episodes {
			eps[i] = fmt.Sprintf("E%02d", e)
		}
		return fmt.Sprintf("%q S%02d%s", p.ShowTitle, ei.Season, strings.Join(eps, ""))
	}
	if ei.IsSeasonPack {
		return fmt.Sprintf("%q S%02d (season pack)", p.ShowTitle, ei.Season)
	}
	return fmt.Sprintf("%q (no episode info)", p.ShowTitle)
}
