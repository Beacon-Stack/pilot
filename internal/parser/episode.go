package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// EpisodeInfo holds parsed season/episode data from a release title.
type EpisodeInfo struct {
	Season          int    // 0 if not found
	Episodes        []int  // empty if season pack or daily/anime only
	AbsoluteEpisode int    // for anime absolute numbering
	AirDate         string // "2024-01-15" for daily shows
	IsSeasonPack    bool   // true if "S01" with no episode number
	IsDaily         bool
	IsAnime         bool
}

var (
	// Standard: S01E05 or multi-episode S01E05E06 or S01E05-E06
	reStdEpisode = regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,3})(?:[-E]E?\d{1,3})*`)

	// All episode numbers within a matched SxxExx token.
	reEpisodeNum = regexp.MustCompile(`(?i)E(\d{1,3})`)

	// Season only: S## (no episode component).  We check post-match that
	// no E## follows, since Go's RE2 doesn't support negative lookahead.
	reSeasonOnly = regexp.MustCompile(`(?i)(S\d{1,2})`)

	// Daily: 2024.01.15, 2024-01-15, or "2024 01 15" (after dot-normalization).
	// The space separator is the normalized form of dots/underscores.
	reDaily = regexp.MustCompile(`(\d{4})[.\- ](\d{2})[.\- ](\d{2})`)

	// Anime bracket group marker at the very start: [Group] or (Group)
	reAnimeBracketGroup = regexp.MustCompile(`^[\[\(][A-Za-z0-9][^\]\)]*[\]\)]`)

	// Anime absolute episode: " - 05" or " - 123" (2-4 digits after " - ")
	reAnimeAbsolute = regexp.MustCompile(`(?:^|\s)-\s*(\d{2,4})(?:\s|\[|$)`)

	// Used to confirm an S## match is NOT immediately followed by E##.
	reEpisodeFollow = regexp.MustCompile(`(?i)^S\d{1,2}E\d`)

	// stripRe matches tokens to remove before title extraction.
	// Season-pack (bare S##) is handled by a separate pass in stripEpisodeTokens
	// so we do not need a lookahead here.
	episodeStripRe = regexp.MustCompile(
		`(?i)` +
			`(?:` +
			// Standard + multi-episode: S01E05, S01E05E06, S01E05-E06
			`S\d{1,2}E\d{1,3}(?:[-E]E?\d{1,3})*` +
			`|` +
			// Daily: YYYY.MM.DD / YYYY-MM-DD / YYYY MM DD (dot, hyphen, or space)
			`\d{4}[.\- ]\d{2}[.\- ]\d{2}` +
			`|` +
			// Anime absolute: " - 05" / " - 123"
			`\s-\s*\d{2,4}(?:\s|\[|$)` +
			`)`,
	)

	// seasonOnlyStripRe matches bare S## tokens that are not part of a SxxExx
	// string.  We strip these separately after the broader episode strip.
	seasonOnlyStripRe = regexp.MustCompile(`(?i)\bS\d{1,2}\b`)
)

// ParseEpisodeInfo extracts TV episode metadata from a raw release title.
// It tries each format in priority order and returns on the first match.
func ParseEpisodeInfo(input string) EpisodeInfo {
	// Normalize separators for matching (dots/underscores → spaces).
	norm := strings.NewReplacer(".", " ", "_", " ").Replace(input)

	// 1. Standard SxxExx (also handles multi-episode S01E05E06 / S01E05-E06)
	if m := reStdEpisode.FindString(norm); m != "" {
		season, episodes := parseStdEpisodeToken(m)
		return EpisodeInfo{
			Season:   season,
			Episodes: episodes,
		}
	}

	// 2. Season pack: S## with no E## anywhere in the token.
	//    We search for the first S## and verify the match isn't part of SxxExx.
	if m := reSeasonOnly.FindStringIndex(norm); m != nil {
		token := norm[m[0]:]
		if !reEpisodeFollow.MatchString(token) {
			sNum := reSeasonOnly.FindStringSubmatch(norm)
			s, _ := strconv.Atoi(sNum[1][1:]) // strip leading 'S'
			return EpisodeInfo{
				Season:       s,
				IsSeasonPack: true,
			}
		}
	}

	// 3. Daily show: YYYY.MM.DD or YYYY-MM-DD
	if m := reDaily.FindStringSubmatch(norm); m != nil {
		airDate := fmt.Sprintf("%s-%s-%s", m[1], m[2], m[3])
		return EpisodeInfo{
			AirDate: airDate,
			IsDaily: true,
		}
	}

	// 4. Anime absolute numbering — only when a leading bracket group is present.
	if reAnimeBracketGroup.MatchString(input) {
		if m := reAnimeAbsolute.FindStringSubmatch(norm); m != nil {
			ep, _ := strconv.Atoi(m[1])
			return EpisodeInfo{
				AbsoluteEpisode: ep,
				IsAnime:         true,
			}
		}
	}

	return EpisodeInfo{}
}

// parseStdEpisodeToken extracts the season number and all episode numbers from
// a matched SxxExx token such as "S01E05", "S01E05E06", or "S01E05-E06".
func parseStdEpisodeToken(token string) (season int, episodes []int) {
	upper := strings.ToUpper(token)

	sIdx := strings.Index(upper, "S")
	eIdx := strings.Index(upper, "E")
	if sIdx < 0 || eIdx < 0 {
		return 0, nil
	}
	season, _ = strconv.Atoi(upper[sIdx+1 : eIdx])

	for _, em := range reEpisodeNum.FindAllStringSubmatch(upper, -1) {
		ep, _ := strconv.Atoi(em[1])
		episodes = append(episodes, ep)
	}
	return season, episodes
}

// stripEpisodeTokens removes episode/season markers from s so that title
// extraction does not include them.
func stripEpisodeTokens(s string) string {
	// First pass: strip standard episode tokens, daily dates, and anime markers.
	result := episodeStripRe.ReplaceAllString(s, " ")
	// Second pass: strip bare season tokens (S01, S02, ...).
	result = seasonOnlyStripRe.ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}
