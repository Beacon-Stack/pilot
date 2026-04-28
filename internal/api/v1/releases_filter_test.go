package v1

import (
	"testing"

	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ⚠ Regression guard for the wrong-torrent bug. If these fail, auto-search
// can once again grab releases that belong to a completely different show
// whose name happens to contain a word in common with the target series.

func titles(rs []indexer.SearchResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Title
	}
	return out
}

func mkResults(titles ...string) []indexer.SearchResult {
	out := make([]indexer.SearchResult, len(titles))
	for i, t := range titles {
		out[i] = indexer.SearchResult{Release: plugin.Release{Title: t}}
	}
	return out
}

func TestFilterByEpisode_DropsUnrelatedSeasonPacks(t *testing.T) {
	results := mkResults(
		"Breaking.Bad.S01.1080p.BluRay.x264",  // ✓
		"Breaking.Bad.Bulgaria.S01.720p.WEB",  // ✗ unrelated
		"The.Breaking.Bad.S01.HDTV",           // ✗ unrelated
		"Breaking.Bad.2008.S01.REPACK.1080p",  // ✓ (trailing year)
		"Breaking.Bad.S02.1080p.BluRay",       // ✗ wrong season
		"Breaking.Bad.S01E05.1080p.HDTV",      // ✓ individual episode, season matches
		"Completely.Unrelated.Show.S01.1080p", // ✗
	)

	got := filterByEpisode(results, "Breaking Bad", nil, 1, 0, 0)

	wantTitles := map[string]bool{
		"Breaking.Bad.S01.1080p.BluRay.x264": true,
		"Breaking.Bad.2008.S01.REPACK.1080p": true,
		"Breaking.Bad.S01E05.1080p.HDTV":     true,
	}
	if len(got) != len(wantTitles) {
		t.Fatalf("filterByEpisode: got %d, want %d — got titles: %v",
			len(got), len(wantTitles), titles(got))
	}
	for _, r := range got {
		if !wantTitles[r.Title] {
			t.Errorf("filterByEpisode kept unexpected release %q", r.Title)
		}
	}
}

func TestFilterByEpisode_EpisodeLevel(t *testing.T) {
	results := mkResults(
		"Breaking.Bad.S01E05.1080p", // ✓
		"Breaking.Bad.S01E06.1080p", // ✗ wrong episode
		"Breaking.Bad.S01.1080p",    // ✓ season pack covers the episode
		"The.Office.S01E05.1080p",   // ✗ unrelated show
	)

	got := filterByEpisode(results, "Breaking Bad", nil, 1, 5, 0)

	want := []string{
		"Breaking.Bad.S01E05.1080p",
		"Breaking.Bad.S01.1080p",
	}
	if len(got) != len(want) {
		t.Fatalf("episode-level filter: got %d, want %d — %v", len(got), len(want), titles(got))
	}
}

func TestFilterByEpisode_SeasonZeroKeepsTitleMatch(t *testing.T) {
	// season=0 means "whole-series search" — skip season/episode gates, but
	// still enforce the title match. Releases without a parseable season
	// marker produce noisy ShowTitle extraction, so this path is used only
	// for inputs that do carry SxxExx-style markers.
	results := mkResults(
		"Breaking.Bad.S01.1080p.BluRay",
		"Breaking.Bad.S02E03.720p",
		"Totally.Different.S01.1080p",
	)
	got := filterByEpisode(results, "Breaking Bad", nil, 0, 0, 0)
	if len(got) != 2 {
		t.Fatalf("season=0 filter: got %d, want 2 — %v", len(got), titles(got))
	}
	for _, r := range got {
		if r.Title == "Totally.Different.S01.1080p" {
			t.Errorf("unrelated show leaked through season=0 filter")
		}
	}
}

func TestFilterByEpisode_WrongSeasonDropped(t *testing.T) {
	got := filterByEpisode(mkResults("Breaking.Bad.S03.1080p"), "Breaking Bad", nil, 1, 0, 0)
	if len(got) != 0 {
		t.Errorf("wrong season: got %v, want empty", titles(got))
	}
}

func TestBuildEpisodeQueries_SeasonEmitsBothForms(t *testing.T) {
	// Regression guard for Sonarr issue #3934 — many torznab indexers
	// tag releases under exactly one naming convention ("S01" vs
	// "Season 1"), so pilot must search for both.
	got := buildEpisodeQueries("Breaking Bad", 1, 0, 0, false)
	want := []string{"Breaking Bad S01", "Breaking Bad Season 1"}
	if len(got) != len(want) {
		t.Fatalf("season query count: got %d, want %d — %v", len(got), len(want), got)
	}
	for i, q := range want {
		if got[i] != q {
			t.Errorf("query %d: got %q, want %q", i, got[i], q)
		}
	}
}

func TestBuildEpisodeQueries_EpisodeIsSingleQuery(t *testing.T) {
	got := buildEpisodeQueries("Breaking Bad", 1, 5, 0, false)
	if len(got) != 1 || got[0] != "Breaking Bad S01E05" {
		t.Errorf("episode query: got %v, want [Breaking Bad S01E05]", got)
	}
}

func TestBuildEpisodeQueries_WholeSeriesIsTitleOnly(t *testing.T) {
	got := buildEpisodeQueries("Breaking Bad", 0, 0, 0, false)
	if len(got) != 1 || got[0] != "Breaking Bad" {
		t.Errorf("whole-series query: got %v, want [Breaking Bad]", got)
	}
}

// Headline regression for the Jujutsu Kaisen incident: a non-trivial
// anime episode produces THREE queries — the standard S01E48 form and
// two absolute-numbering forms ("48" with and without dash). Without
// these absolute forms, search returns zero results because anime
// fansubs tag releases as "Show - 48", not "Show S01E48".
func TestBuildEpisodeQueries_AnimeEmitsAbsoluteForms(t *testing.T) {
	got := buildEpisodeQueries("Jujutsu Kaisen", 1, 48, 48, true)
	want := []string{
		"Jujutsu Kaisen S01E48",
		"Jujutsu Kaisen 48",
		"Jujutsu Kaisen - 48",
	}
	if len(got) != len(want) {
		t.Fatalf("anime query count: got %d, want %d — %v", len(got), len(want), got)
	}
	for i, q := range want {
		if got[i] != q {
			t.Errorf("query %d: got %q, want %q", i, got[i], q)
		}
	}
}

// Single-digit absolute episode numbers must still be zero-padded so
// "Show 03" matches indexer titles like "Show - 03 [1080p]".
func TestBuildEpisodeQueries_AnimeAbsoluteIsZeroPadded(t *testing.T) {
	got := buildEpisodeQueries("Jujutsu Kaisen", 1, 3, 3, true)
	want := []string{
		"Jujutsu Kaisen S01E03",
		"Jujutsu Kaisen 03",
		"Jujutsu Kaisen - 03",
	}
	for i, q := range want {
		if i >= len(got) || got[i] != q {
			t.Errorf("query %d: got %q, want %q", i, got[i], q)
		}
	}
}

// Anime augmentation must NOT fire when absolute is 0 — that's the
// "absolute number unknown" sentinel. Falling through to standard
// queries is the right behavior; emitting "Show 00" would be garbage.
func TestBuildEpisodeQueries_AnimeWithUnknownAbsoluteFallsBack(t *testing.T) {
	got := buildEpisodeQueries("Jujutsu Kaisen", 1, 48, 0, true)
	if len(got) != 1 || got[0] != "Jujutsu Kaisen S01E48" {
		t.Errorf("missing absolute should fall back to S01E48 only; got %v", got)
	}
}

// isAnime=false forces the standard path even when absolute is set.
// Guards against accidentally upgrading regular Western TV to anime
// queries (which would just be noise).
func TestBuildEpisodeQueries_NonAnimeIgnoresAbsolute(t *testing.T) {
	got := buildEpisodeQueries("Breaking Bad", 1, 5, 5, false)
	if len(got) != 1 || got[0] != "Breaking Bad S01E05" {
		t.Errorf("non-anime should ignore absolute; got %v", got)
	}
}

// Season-pack searches don't get absolute augmentation either —
// "Show 48" would mean "episode 48", not "season 1 pack".
func TestBuildEpisodeQueries_AnimeSeasonPackUnchanged(t *testing.T) {
	got := buildEpisodeQueries("Jujutsu Kaisen", 1, 0, 0, true)
	want := []string{"Jujutsu Kaisen S01", "Jujutsu Kaisen Season 1"}
	if len(got) != len(want) {
		t.Fatalf("season pack query count for anime: got %d, want %d — %v", len(got), len(want), got)
	}
}

func TestApplyQualityProfile_ResolutionFloor(t *testing.T) {
	profile := &quality.Profile{
		Cutoff: plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceBluRay, Codec: plugin.CodecX265},
	}

	results := []indexer.SearchResult{
		// 1080p releases with various sources/codecs — all should pass the
		// resolution floor because resolution >= 1080p regardless of the
		// exact source/codec combo in the profile's allowed list.
		{Release: plugin.Release{Title: "Show.S01.1080p.BluRay.x265",
			Quality: plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceBluRay, Codec: plugin.CodecX265}}},
		{Release: plugin.Release{Title: "Show.S01.1080p.WEBDL.x264",
			Quality: plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL, Codec: plugin.CodecX264}}},
		{Release: plugin.Release{Title: "Show.S01.2160p.BluRay.x265",
			Quality: plugin.Quality{Resolution: plugin.Resolution2160p, Source: plugin.SourceBluRay, Codec: plugin.CodecX265}}},
		// 720p — below floor, should be tagged.
		{Release: plugin.Release{Title: "Show.S01.720p.BluRay.x264",
			Quality: plugin.Quality{Resolution: plugin.Resolution720p, Source: plugin.SourceBluRay, Codec: plugin.CodecX264}}},
		// 480p — below floor, should be tagged.
		{Release: plugin.Release{Title: "Show.S01.480p.DVD.x264",
			Quality: plugin.Quality{Resolution: plugin.Resolution480p, Source: plugin.SourceDVD, Codec: plugin.CodecX264}}},
	}

	applyQualityProfile(results, profile)

	wantTagged := map[string]bool{
		"Show.S01.720p.BluRay.x264": true,
		"Show.S01.480p.DVD.x264":    true,
	}
	for _, r := range results {
		hasTag := false
		for _, reason := range r.FilterReasons {
			if reason == "below_quality_profile" {
				hasTag = true
				break
			}
		}
		if wantTagged[r.Title] && !hasTag {
			t.Errorf("%q should have been tagged below_quality_profile", r.Title)
		}
		if !wantTagged[r.Title] && hasTag {
			t.Errorf("%q was tagged below_quality_profile but should pass the floor", r.Title)
		}
	}
}

// ── Alternate titles ────────────────────────────────────────────────────────

// The headline alt-title regression case. Indexers respond with releases
// that use marketing/regional names for "Andor" — when the series carries
// those names as alternates (from TMDB), they must pass the title gate.
func TestFilterByEpisode_AlternateTitleUnlocksReleases(t *testing.T) {
	results := mkResults(
		"Star.Wars.Andor.S01.1080p.BluRay",  // ✓ via alternate
		"Andor.S01.1080p.BluRay",            // ✓ canonical
		"Andor.A.Star.Wars.Story.S01.720p",  // ✓ via alternate
		"Mandor.S01.1080p",                  // ✗ different show
	)
	alts := []string{"Star Wars: Andor", "Andor: A Star Wars Story"}

	got := filterByEpisode(results, "Andor", alts, 1, 0, 0)

	want := map[string]bool{
		"Star.Wars.Andor.S01.1080p.BluRay":   true,
		"Andor.S01.1080p.BluRay":             true,
		"Andor.A.Star.Wars.Story.S01.720p":   true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d, want %d — %v", len(got), len(want), titles(got))
	}
	for _, r := range got {
		if !want[r.Title] {
			t.Errorf("unexpected release kept: %q", r.Title)
		}
	}
}

// Without alternates configured, the alt-named releases must still drop —
// proves we haven't accidentally relaxed the strict gate globally.
func TestFilterByEpisode_NoAlternatesPreservesStrictness(t *testing.T) {
	results := mkResults(
		"Star.Wars.Andor.S01.1080p", // strict-rule reject — no alt
		"Andor.S01.1080p",            // canonical, accept
	)
	got := filterByEpisode(results, "Andor", nil, 1, 0, 0)

	if len(got) != 1 || got[0].Title != "Andor.S01.1080p" {
		t.Errorf("strict mode kept wrong set: %v", titles(got))
	}
}

// Empty-slice alternates (loaded from a series row with `[]`) must
// behave identically to nil alternates. Avoids a regression where
// empty-slice handling differs from nil.
func TestFilterByEpisode_EmptyAlternateListSameAsNil(t *testing.T) {
	results := mkResults("Star.Wars.Andor.S01.1080p")
	gotNil := filterByEpisode(results, "Andor", nil, 1, 0, 0)
	gotEmpty := filterByEpisode(results, "Andor", []string{}, 1, 0, 0)
	if len(gotNil) != len(gotEmpty) {
		t.Errorf("nil alts and []string{} produced different results: %d vs %d",
			len(gotNil), len(gotEmpty))
	}
}

// Alternate titles must NOT bypass season/episode gating. A release
// matching an alternate but for the wrong season still drops.
func TestFilterByEpisode_AlternateTitleRespectsSeasonGate(t *testing.T) {
	results := mkResults(
		"Star.Wars.Andor.S01.1080p", // ✓ via alt, S01 matches
		"Star.Wars.Andor.S02.1080p", // ✗ via alt but wrong season
	)
	got := filterByEpisode(results, "Andor", []string{"Star Wars: Andor"}, 1, 0, 0)
	if len(got) != 1 || got[0].Title != "Star.Wars.Andor.S01.1080p" {
		t.Errorf("season gate didn't fire on alternate-title match: %v", titles(got))
	}
}

// Alternate that overlaps with the canonical (caller mistakenly added
// the canonical to the alt list too) should not double-match or break.
func TestFilterByEpisode_AlternateContainingCanonicalIsHarmless(t *testing.T) {
	results := mkResults("Andor.S01.1080p")
	got := filterByEpisode(results, "Andor", []string{"Andor", "Star Wars: Andor"}, 1, 0, 0)
	if len(got) != 1 {
		t.Errorf("dedup behavior off: got %d results, want 1 (%v)", len(got), titles(got))
	}
}

func TestApplyQualityProfile_NilProfileIsNoOp(t *testing.T) {
	results := []indexer.SearchResult{
		{Release: plugin.Release{Title: "Show.S01.480p",
			Quality: plugin.Quality{Resolution: plugin.Resolution480p}}},
	}
	applyQualityProfile(results, nil)
	if len(results[0].FilterReasons) != 0 {
		t.Errorf("nil profile should not tag anything, got %v", results[0].FilterReasons)
	}
}
