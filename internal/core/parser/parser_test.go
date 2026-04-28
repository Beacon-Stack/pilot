package parser

import "testing"

func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Breaking Bad", "breakingbad"},
		{"Breaking.Bad", "breakingbad"},
		{"breaking_bad", "breakingbad"},
		{"Breaking-Bad!", "breakingbad"},
		{"Breaking Bad (2008)", "breakingbad2008"},
		{"", ""},
		{"   ", ""},
		{"S.H.I.E.L.D.", "shield"},
	}
	for _, tc := range cases {
		if got := NormalizeTitle(tc.in); got != tc.want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTitleMatches_ExactAndPunctuation(t *testing.T) {
	accept := []struct{ series, release string }{
		{"Breaking Bad", "Breaking Bad"},
		{"Breaking Bad", "Breaking.Bad"},
		{"Breaking Bad", "breaking bad"},
		{"Breaking Bad", "Breaking-Bad"},
		{"Raised by Wolves", "Raised.by.Wolves"},
		{"Mr. Robot", "Mr Robot"},
	}
	for _, tc := range accept {
		if !TitleMatches(tc.series, tc.release) {
			t.Errorf("TitleMatches(%q, %q) = false, want true", tc.series, tc.release)
		}
	}
}

func TestTitleMatches_TrailingYearAllowed(t *testing.T) {
	// Indexers routinely embed the premiere year in release names.
	if !TitleMatches("Breaking Bad", "Breaking Bad 2008") {
		t.Error("trailing year should match")
	}
	if !TitleMatches("Breaking Bad", "Breaking.Bad.2008") {
		t.Error("trailing year after dot separator should match")
	}
	if !TitleMatches("The Office", "The Office 1999") {
		t.Error("trailing year with 'The' prefix should match")
	}
}

func TestTitleMatches_RejectsUnrelated(t *testing.T) {
	// These are the regression cases. Each is a release that would have
	// slipped past the old filter because it shares a season number and
	// parses as a season pack, but is the wrong show.
	reject := []struct{ series, release string }{
		{"Breaking Bad", "Breaking Bad Bulgaria"},
		{"Breaking Bad", "The Breaking Bad"},
		{"The Office", "The Office US"}, // strict per user directive
		{"The Office", "The Office UK"},
		{"Bad", "Breaking Bad"}, // "Bad" must not match "Breaking Bad"
		{"Breaking Bad", "Breaking"},
		{"Breaking Bad", "Totally Unrelated"},
		{"Breaking Bad", ""},
		{"", "Breaking Bad"},
	}
	for _, tc := range reject {
		if TitleMatches(tc.series, tc.release) {
			t.Errorf("TitleMatches(%q, %q) = true, want false", tc.series, tc.release)
		}
	}
}

func TestTitleMatchesAny_AlternateTitlesUnlockMatches(t *testing.T) {
	// The headline alternate-titles regression case: indexers respond to
	// "Andor" searches with "Star Wars Andor" releases. The strict gate
	// rejects them; an "Andor" series with the TMDB-supplied alternate
	// "Star Wars: Andor" should accept them via TitleMatchesAny.
	candidates := []string{"Andor", "Star Wars: Andor", "Andor: A Star Wars Story"}
	accept := []string{
		"Star Wars Andor",
		"Star.Wars.Andor",
		"Andor",
		"Andor A Star Wars Story",
	}
	for _, rel := range accept {
		if !TitleMatchesAny(candidates, rel) {
			t.Errorf("TitleMatchesAny(%v, %q) = false, want true", candidates, rel)
		}
	}
}

func TestTitleMatchesAny_AlternateTitlesDoNotBypassStrictness(t *testing.T) {
	// The wrong-torrent guard from TestTitleMatches_RejectsUnrelated must
	// still hold — adding alternate titles is per-candidate strict, not
	// fuzzy. So an "Andor" series with alternate "Star Wars: Andor"
	// still rejects "Mandor" or "Star Wars Andor Bloopers".
	candidates := []string{"Andor", "Star Wars: Andor"}
	reject := []string{
		"Mandor",
		"Andores",
		"Star Wars Andor Behind The Scenes Special",
		"Big Bang Theory", // unrelated
	}
	for _, rel := range reject {
		if TitleMatchesAny(candidates, rel) {
			t.Errorf("TitleMatchesAny(%v, %q) = true, want false", candidates, rel)
		}
	}
}

func TestTitleMatchesAny_EmptyCandidatesNeverMatch(t *testing.T) {
	if TitleMatchesAny(nil, "anything") {
		t.Error("nil candidates should never match")
	}
	if TitleMatchesAny([]string{}, "anything") {
		t.Error("empty candidates should never match")
	}
}

func TestTitleMatches_YearNot4Digits(t *testing.T) {
	// Anything past a 4-digit-year suffix must be rejected.
	if TitleMatches("Breaking Bad", "Breaking Bad 20081") {
		t.Error("5-digit suffix should not count as a year")
	}
	if TitleMatches("Breaking Bad", "Breaking Bad 99") {
		t.Error("2-digit suffix should not count as a year")
	}
}

// ── Anime absolute-episode parsing ─────────────────────────────────────────

// Headline regression for the Jujutsu Kaisen incident: fansub release
// titles use absolute numbering ("Show - 48"). Without recognizing
// these, filterByEpisode drops every search hit because the parsed
// season is 0 instead of the user's requested season.
func TestParse_AnimeAbsoluteEpisode_DashSeparated(t *testing.T) {
	cases := []struct {
		title        string
		wantTitle    string
		wantAbsolute int
	}{
		{"[SubsPlease] Jujutsu Kaisen - 48 [1080p].mkv", "[SubsPlease] Jujutsu Kaisen", 48},
		{"Jujutsu.Kaisen.-.48.1080p.WEB-DL", "Jujutsu Kaisen", 48},
		{"Show Title - 03 [720p]", "Show Title", 3},
		{"Show - 100 [1080p]", "Show", 100},
		{"Show - 48v2 [1080p].mkv", "Show", 48}, // v2 re-release
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			p := Parse(c.title)
			if p.EpisodeInfo.AbsoluteEpisode != c.wantAbsolute {
				t.Errorf("absolute: got %d, want %d", p.EpisodeInfo.AbsoluteEpisode, c.wantAbsolute)
			}
		})
	}
}

// SxxExx form takes precedence for season/episode, but explicit
// parenthesized absolute markers ride along. The release "Jujutsu
// Kaisen S03E01 (E48)" should parse as season=3 episode=1 *and*
// absolute=48 — that absolute is what lets filterByEpisode match the
// user's "give me TMDB-relative S01E48" search against TVDB-tagged
// releases.
func TestParse_SxxExxWithExplicitAbsoluteMarker(t *testing.T) {
	p := Parse("Jujutsu Kaisen   S03E01 (E48) [H3LL][1080p][x264][10bit][AAC][Multi-Subs]")
	if p.EpisodeInfo.Season != 3 {
		t.Errorf("season: got %d, want 3", p.EpisodeInfo.Season)
	}
	if len(p.EpisodeInfo.Episodes) != 1 || p.EpisodeInfo.Episodes[0] != 1 {
		t.Errorf("episodes: got %v, want [1]", p.EpisodeInfo.Episodes)
	}
	if p.EpisodeInfo.AbsoluteEpisode != 48 {
		t.Errorf("absolute: got %d, want 48 (from explicit (E48) marker)", p.EpisodeInfo.AbsoluteEpisode)
	}
}

// Bare SxxExx without a (Eabs) marker leaves AbsoluteEpisode at zero —
// guards against accidentally inferring an absolute from random digits.
func TestParse_SxxExxWithoutAbsoluteMarker(t *testing.T) {
	p := Parse("Show.S01E48.1080p.WEB-DL")
	if p.EpisodeInfo.Season != 1 {
		t.Errorf("season: got %d, want 1", p.EpisodeInfo.Season)
	}
	if len(p.EpisodeInfo.Episodes) != 1 || p.EpisodeInfo.Episodes[0] != 48 {
		t.Errorf("episodes: got %v, want [48]", p.EpisodeInfo.Episodes)
	}
	if p.EpisodeInfo.AbsoluteEpisode != 0 {
		t.Errorf("no (Eabs) marker → absolute must stay 0; got %d", p.EpisodeInfo.AbsoluteEpisode)
	}
}

// Year must NOT be parsed as an absolute episode. "Show - 2024" looks
// like the regex shape but the digits overlap the year and we reject.
func TestParse_DoesNotConfuseYearWithAbsolute(t *testing.T) {
	p := Parse("Some Show - 2024")
	if p.EpisodeInfo.AbsoluteEpisode != 0 {
		t.Errorf("year-shaped digits must not parse as absolute; got %d", p.EpisodeInfo.AbsoluteEpisode)
	}
	if p.Year != 2024 {
		t.Errorf("year must still parse: got %d", p.Year)
	}
}

// Resolution markers (1080p, 720p) must NOT be parsed as absolute.
// The trailing "p" exclusion in the regex prevents this.
func TestParse_DoesNotConfuseResolutionWithAbsolute(t *testing.T) {
	p := Parse("Show - 1080p [WEB]")
	if p.EpisodeInfo.AbsoluteEpisode != 0 {
		t.Errorf("resolution token must not parse as absolute; got %d", p.EpisodeInfo.AbsoluteEpisode)
	}
}

// Fansub releases prepend [group] tags to titles. Stripping them in
// cleanTitle is what lets canonical TMDB titles ("Jujutsu Kaisen")
// match against fansub release titles ("[SubsPlease] Jujutsu Kaisen").
// Without this, every Nyaa.si result fails the title gate.
func TestParse_StripsLeadingGroupPrefixes(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"[SubsPlease] Jujutsu Kaisen - 48 (1080p) [hash].mkv", "Jujutsu Kaisen"},
		{"[Erai-raws] Jujutsu Kaisen - 48 [1080p][Multi-Subs].mkv", "Jujutsu Kaisen"},
		{"[Erai-raws][SubsPlease] Show - 12 [1080p].mkv", "Show"},
		{"Jujutsu Kaisen - 48 [1080p]", "Jujutsu Kaisen"}, // no prefix to strip
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			got := Parse(c.title).ShowTitle
			if got != c.want {
				t.Errorf("show title: got %q, want %q", got, c.want)
			}
		})
	}
}

// Edge case: a malformed prefix without a closing bracket must NOT
// eat the entire title (would happen with `s = s[end+1:]` if end was
// negative). Defensive against indexer titles that break our regex
// assumptions.
func TestParse_MalformedBracketLeavesTitleAlone(t *testing.T) {
	got := Parse("[no-closing-bracket Jujutsu Kaisen - 48").ShowTitle
	if got == "" {
		t.Error("malformed bracket prefix wiped the title")
	}
}
