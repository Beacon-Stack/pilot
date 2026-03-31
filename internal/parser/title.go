package parser

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// stopTokens marks the boundary between title and quality metadata.
var stopTokens = []string{
	// TV episode markers — must come first so they stop title extraction early
	`S\d\d`,
	// resolutions / scan type
	"2160p", "1080p", "1080i", "720p", "720i", "576p", "480p", "4k", "uhd",
	// source
	"bluray", "blu-ray", "bdrip", "brrip", "dvdrip", "dvdscr", "dvd",
	"webrip", "web-rip", "web-dl", "webdl", "web", "hdtv", "hdcam", "cam",
	"hdrip", "vhsrip", "ts", "hc", "r5",
	// video codec
	"x264", "x265", "h264", "h265", "hevc", "avc", "xvid", "divx",
	"10bit", "10-bit", "hdr", "hdr10", "dolbyvision", "dv",
	// audio codec
	"dts", "dts-hd", "dtshd", "dts-x", "dtsx", "truehd", "atmos",
	"ddplus", "ddp", "dd", "ac3", "aac", "mp3", "flac", "eac3",
	"lpcm", "pcm", "opus",
	// channels
	"7.1", "5.1", "2.0",
	// misc release flags
	"repack", "proper", "extended", "theatrical", "directors", "dc",
	"unrated", "retail", "readnfo", "nfofix", "limited", "complete",
	"remux", "imax", "hybrid",
}

var yearRe = regexp.MustCompile(`\b((?:19|20)\d{2})\b`)

// stopRe is built from stopTokens. The S\d\d entry is a raw regex pattern
// (not escaped) so it matches season markers like S01, S02, etc.
var stopRe = func() *regexp.Regexp {
	parts := make([]string, len(stopTokens))
	for i, t := range stopTokens {
		// The season-marker token is already a regex pattern; escape all others.
		if t == `S\d\d` {
			parts[i] = t
		} else {
			parts[i] = regexp.QuoteMeta(t)
		}
	}
	return regexp.MustCompile(`(?i)(?:` + strings.Join(parts, "|") + `)`)
}()

// discNoiseRe matches MakeMKV disc-ripper labels.
var discNoiseRe = regexp.MustCompile(`(?i)\b(?:Title|Chapter|Disc|Track)\d+\b`)

// ptRe normalises "Pt1" / "Pt.2" → "Part 1" / "Part 2".
var ptRe = regexp.MustCompile(`(?i)\bPt\.?\s*(\d+)\b`)

const allCapsThreshold = 0.6

// extractTitle parses the movie/show title and year from a normalized string.
func extractTitle(normalized string) (title string, year int) {
	// Find earliest stop token.
	stopIdx := len(normalized)
	if m := stopRe.FindStringIndex(normalized); m != nil {
		stopIdx = m[0]
	}

	// Find the last year at or before the stop position.
	yearCutAt := stopIdx
	for _, m := range yearRe.FindAllStringIndex(normalized, -1) {
		if m[0] > stopIdx {
			break
		}
		if y, err := strconv.Atoi(normalized[m[0]:m[1]]); err == nil {
			year = y
			yearCutAt = m[0]
		}
	}

	rawTitle := strings.TrimSpace(normalized[:yearCutAt])
	if rawTitle == "" {
		// Year-titled movie (e.g. "1917 2019 1080p"): use everything
		// before the first stop token as the title and drop the year.
		rawTitle = strings.TrimSpace(normalized[:stopIdx])
		year = 0
	}

	rawTitle = strings.TrimRight(rawTitle, " -([{")
	title = toTitleCase(rawTitle)
	return title, year
}

// ParseTitle is a public helper that extracts the show/movie title and year
// from a raw filename or release name.
func ParseTitle(name string) (title string, year int) {
	return extractTitle(normalize(name))
}

// normalize prepares a raw release name for parsing.
func normalize(name string) string {
	// Strip path prefix.
	if idx := strings.LastIndexAny(name, `/\`); idx >= 0 {
		name = name[idx+1:]
	}

	// Strip video file extension.
	if dot := strings.LastIndex(name, "."); dot > 0 {
		ext := strings.ToLower(name[dot+1:])
		videoExts := map[string]bool{
			"mkv": true, "mp4": true, "avi": true, "mov": true,
			"wmv": true, "m4v": true, "ts": true, "m2ts": true,
			"flv": true, "webm": true,
		}
		if videoExts[ext] {
			name = name[:dot]
		}
	}

	// Replace separators with spaces.
	normalized := strings.Map(func(r rune) rune {
		if r == '.' || r == '_' {
			return ' '
		}
		return r
	}, name)
	normalized = strings.Join(strings.Fields(normalized), " ")

	// Remove disc-ripper noise.
	normalized = discNoiseRe.ReplaceAllString(normalized, " ")

	// Normalize Pt abbreviations.
	normalized = ptRe.ReplaceAllStringFunc(normalized, func(m string) string {
		digits := ptRe.FindStringSubmatch(m)
		if len(digits) < 2 {
			return m
		}
		return "Part " + digits[1]
	})

	normalized = strings.Join(strings.Fields(normalized), " ")

	// All-caps detection.
	upper, total := 0, 0
	for _, r := range normalized {
		if unicode.IsLetter(r) {
			total++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if total > 0 && float64(upper)/float64(total) >= allCapsThreshold {
		normalized = strings.ToLower(normalized)
	}

	return normalized
}

func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		runes := []rune(w)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}
