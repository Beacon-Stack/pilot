package parser

import (
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

	// AbsoluteEpisode is the parsed absolute episode number from anime-
	// style titles ("[Group] Show - 48 [1080p]" → 48). Zero when the
	// title doesn't match the absolute-pattern OR when SxxExx took
	// precedence (the SxxExx form is preferred whenever both could
	// apply, since it's less ambiguous).
	AbsoluteEpisode int
}

// episodeParse is what parseEpisode finds in a name.
type episodeParse struct {
	Title       string
	Year        int
	EpisodeInfo EpisodeInfo
}

// seEpisodeRe matches patterns like S01E02, S01E02E03, s1e2, etc.
// Group 1: season; groups 2…n: episode numbers (one per "E\d+" token).
var seEpisodeRe = regexp.MustCompile(`(?i)[._\s-]S(\d{1,2})((?:E\d{1,3})+)`)

// seasonPackRe matches a bare season marker with no episode number, in
// both the compact form (S02) and the word form (Season 2 / Season.2).
// The word form matters: Pilot deliberately issues a "<Title> Season N"
// query variant to find these releases, and dropping them here silently
// discarded every result that query variant produced.
var seasonPackRe = regexp.MustCompile(`(?i)(?:^|[._\s-])(?:S(\d{1,2})|Season[._\s]+(\d{1,2}))(?:[._\s-]|$)`)

// episodeNumsRe extracts individual episode numbers from a token like E01E02E03.
var episodeNumsRe = regexp.MustCompile(`(?i)E(\d{1,3})`)

// yearRe matches a 4-digit year (1900–2099).
var yearRe = regexp.MustCompile(`(?:^|[._\s(-])((?:19|20)\d{2})(?:[._\s)-]|$)`)

// absoluteEpisodeDashRe matches the fansub-standard dash-separated
// form: "<title> - 48 [1080p]", "Title.-.48.1080p", etc. The dash
// anchor + the surrounding separators keep this from firing on years,
// sizes, or trailing release-group dashes.
var absoluteEpisodeDashRe = regexp.MustCompile(`(?i)[._\s][-_][._\s](\d{1,4})(?:v\d+)?(?:[._\s]|$)`)

// absoluteEpisodeParenRe matches an explicit absolute marker in
// parentheses: "S03E01 (E48)". Used when a release tagger preserves
// the absolute number alongside a TVDB-style season split. Only the
// parenthesized form — bare "E48" appears too often in unrelated
// contexts (codec strings, scene tags) to match safely on its own.
var absoluteEpisodeParenRe = regexp.MustCompile(`(?i)\(E(\d{1,4})\)`)

// parseEpisode extracts the show title, year and season/episode position
// from a file name or release title. It is lenient: what can't be
// determined is left at its zero value.
func parseEpisode(filename string) episodeParse {
	// Strip leading path component if present.
	filename = strings.TrimSuffix(filename, "/")
	if idx := strings.LastIndexAny(filename, "/\\"); idx >= 0 {
		filename = filename[idx+1:]
	}
	// Strip the file extension — but only a KNOWN one. Parse also runs on
	// release titles, and blindly truncating at the last dot destroyed
	// titles whose final dot-separated token is short ("S.W.A.T. S05E01"
	// → "S.W.A.T", losing the episode token before any regex ran).
	if dot := strings.LastIndex(filename, "."); dot > 0 {
		if _, ok := knownFileExtensions[strings.ToLower(filename[dot+1:])]; ok {
			filename = filename[:dot]
		}
	}

	var result episodeParse

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
		result.Title = cleanTitle(titlePart)

		// Additive absolute extraction: a release tagger may emit BOTH
		// SxxExx and an explicit (Eabs) marker (e.g. "S03E01 (E48)").
		// When that's present, capture the absolute too — anime-aware
		// filtering downstream uses it to match against the user's
		// requested episode regardless of season layout.
		if am := absoluteEpisodeParenRe.FindStringSubmatch(filename); len(am) >= 2 {
			if n, _ := strconv.Atoi(am[1]); n > 0 {
				result.EpisodeInfo.AbsoluteEpisode = n
			}
		}
		return result
	}

	// ── Season pack (no episode number) ──────────────────────────────────────
	if m := seasonPackRe.FindStringSubmatchIndex(filename); m != nil {
		// Group 1 is the compact form (S02), group 2 the word form
		// (Season 2) — exactly one is set.
		numStart, numEnd := m[2], m[3]
		if numStart < 0 {
			numStart, numEnd = m[4], m[5]
		}
		season, _ := strconv.Atoi(filename[numStart:numEnd])
		result.EpisodeInfo.Season = season
		result.EpisodeInfo.IsSeasonPack = true

		result.Title = cleanTitle(filename[:m[0]])
		return result
	}

	// ── Anime absolute episode number ────────────────────────────────────────
	// Tried only when no SxxExx and no season pack matched. Fansub
	// releases like "[Group] Jujutsu Kaisen - 48 [1080p].mkv" land here.
	// Year / resolution markers are excluded by the regex's shape
	// (mandatory dash, no trailing "p" or extra digits). The year Re
	// already ran above so result.Year is set independently.
	if m := absoluteEpisodeDashRe.FindStringSubmatchIndex(filename); m != nil {
		// Reject the match if it overlaps the parsed year — protects
		// against "Show - 2024" being read as episode 2024 when the
		// real release year token contains the same characters.
		yearMatch := yearRe.FindStringIndex(filename)
		if yearMatch == nil || m[2] >= yearMatch[1] || m[3] <= yearMatch[0] {
			n, _ := strconv.Atoi(filename[m[2]:m[3]])
			if n > 0 {
				result.EpisodeInfo.AbsoluteEpisode = n
				// Title is everything before the " - NN" token.
				result.Title = cleanTitle(filename[:m[0]])
				return result
			}
		}
	}

	// ── No season/episode information found ───────────────────────────────────
	result.Title = cleanTitle(filename)
	return result
}

// knownFileExtensions is the set of extensions Parse strips before
// parsing. Media containers, subtitle/sidecar files, archives, and the
// fake-upload favorite .exe — never bare words or codec tokens ("264" in
// "H.264", "1" in "DD5.1" survive because they're not in the set).
var knownFileExtensions = map[string]struct{}{
	"mkv": {}, "mp4": {}, "avi": {}, "m4v": {}, "mov": {}, "wmv": {},
	"ts": {}, "m2ts": {}, "mpg": {}, "mpeg": {}, "webm": {}, "flv": {},
	"ogm": {}, "divx": {}, "iso": {}, "img": {},
	"rar": {}, "zip": {}, "7z": {},
	"srt": {}, "sub": {}, "idx": {}, "nfo": {}, "torrent": {},
	"mka": {}, "mks": {}, "exe": {},
}
