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

	got := filterByEpisode(results, "Breaking Bad", 1, 0)

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

	got := filterByEpisode(results, "Breaking Bad", 1, 5)

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
	got := filterByEpisode(results, "Breaking Bad", 0, 0)
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
	got := filterByEpisode(mkResults("Breaking.Bad.S03.1080p"), "Breaking Bad", 1, 0)
	if len(got) != 0 {
		t.Errorf("wrong season: got %v, want empty", titles(got))
	}
}

func TestBuildEpisodeQueries_SeasonEmitsBothForms(t *testing.T) {
	// Regression guard for Sonarr issue #3934 — many torznab indexers
	// tag releases under exactly one naming convention ("S01" vs
	// "Season 1"), so pilot must search for both.
	got := buildEpisodeQueries("Breaking Bad", 1, 0)
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
	got := buildEpisodeQueries("Breaking Bad", 1, 5)
	if len(got) != 1 || got[0] != "Breaking Bad S01E05" {
		t.Errorf("episode query: got %v, want [Breaking Bad S01E05]", got)
	}
}

func TestBuildEpisodeQueries_WholeSeriesIsTitleOnly(t *testing.T) {
	got := buildEpisodeQueries("Breaking Bad", 0, 0)
	if len(got) != 1 || got[0] != "Breaking Bad" {
		t.Errorf("whole-series query: got %v, want [Breaking Bad]", got)
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
