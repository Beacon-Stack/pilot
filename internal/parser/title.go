package parser

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// NormalizeTitle lowercases a title and reduces it to letters and digits
// so two titles can be compared without being tripped up by dots,
// underscores, spaces, or punctuation. Unicode-aware: accented letters
// fold to their ASCII base ("Pokémon" → "pokemon", matching release
// titles that drop the accent), non-Latin letters are kept (not deleted
// — deleting them normalized CJK titles to "" and made every comparison
// fail), and "&" folds to "and" so "Rick & Morty" matches
// "Rick.and.Morty".
//
// Example:
//
//	NormalizeTitle("Breaking.Bad") == NormalizeTitle("breaking bad") == "breakingbad"
func NormalizeTitle(s string) string {
	s = strings.ReplaceAll(s, "&", " and ")
	// NFD-decompose so diacritics become combining marks we can drop.
	s = norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// Combining mark (the accent split off by NFD) — drop.
		case unicode.IsLetter(r) || unicode.IsDigit(r):
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
// exact wrong-torrent bug this is meant to prevent. Use TitleMatchesAny
// when the series carries explicit alternate titles (from TMDB).
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

// TitleMatchesAny reports whether a release's parsed show title matches
// any of the supplied candidate titles using the strict TitleMatches
// rules. Pass the canonical series title together with any alternate
// titles (from TMDB).
//
// This is the right call for indexer-result filtering: an "Andor"
// series with alternate "Star Wars: Andor" will match release
// "Star Wars Andor S01..." because one of the candidates ("Star Wars:
// Andor") is itself a strict match. The strict semantics are preserved
// per-candidate — no fuzzy logic is added at the aggregate level.
func TitleMatchesAny(candidates []string, releaseShowTitle string) bool {
	for _, t := range candidates {
		if TitleMatches(t, releaseShowTitle) {
			return true
		}
	}
	return false
}

// cleanTitle replaces dots, underscores, and multiple spaces with a single
// space and trims whitespace from both ends. It also strips leading
// `[group]` prefixes that anime fansubs prepend to release titles
// ("[SubsPlease] Jujutsu Kaisen" → "Jujutsu Kaisen") so the strict
// title gate matches canonical TMDB titles. Only consecutive bracket
// groups at the start are removed; trailing brackets (resolution,
// language, hash) are left alone — the title gate doesn't read them.
func cleanTitle(s string) string {
	s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
	// Strip leading [Group] bracket prefixes. Loop because some fansubs
	// stack them ("[Erai-raws][SubsPlease] Show").
	for {
		s = strings.TrimSpace(s)
		if !strings.HasPrefix(s, "[") {
			break
		}
		end := strings.Index(s, "]")
		if end <= 0 {
			break
		}
		s = s[end+1:]
	}
	// Collapse runs of whitespace.
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	// Trim trailing separator characters left over from the split. A
	// filename like "Show - S01E01 - Title" splits at " S01E01" and
	// leaves "Show -" — the trailing " -" then breaks downstream
	// title-equality lookups for legitimately-hyphenated show names
	// like "Star Wars: Maul - Shadow Lord" (the DB title has no
	// trailing dash).
	return strings.TrimRight(strings.TrimSpace(s), " -._")
}
