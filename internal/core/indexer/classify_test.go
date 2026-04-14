package indexer

import (
	"testing"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ⚠ Regression guard for the season-pack-visibility bug. If these fail, the
// interactive search modal will once again surface individual episodes above
// season packs within the same quality tier, making season packs hard to
// find. See plan: "Tier 1 — surface pack type end-to-end".

func TestClassifyRelease(t *testing.T) {
	cases := []struct {
		title    string
		wantType PackType
		wantN    int
	}{
		{"Breaking.Bad.S01.1080p.BluRay", PackTypeSeason, 0},
		{"Breaking.Bad.S01.2008.REPACK", PackTypeSeason, 0},
		{"Breaking.Bad.S01E05.720p.WEB", PackTypeEpisode, 1},
		{"Breaking.Bad.S01E05E06.1080p", PackTypeMulti, 2},
		{"Breaking.Bad.S01E01E02E03.Complete", PackTypeMulti, 3},
		{"Breaking.Bad.Just.The.Pilot", PackTypeUnknown, 0},
	}
	for _, tc := range cases {
		gotType, gotN := classifyRelease(tc.title)
		if gotType != tc.wantType || gotN != tc.wantN {
			t.Errorf("classifyRelease(%q) = (%v, %d), want (%v, %d)",
				tc.title, gotType, gotN, tc.wantType, tc.wantN)
		}
	}
}

func TestEffectiveEpisodeCount_Ordering(t *testing.T) {
	season := SearchResult{PackType: PackTypeSeason}
	multi5 := SearchResult{PackType: PackTypeMulti, EpisodeCount: 5}
	multi2 := SearchResult{PackType: PackTypeMulti, EpisodeCount: 2}
	episode := SearchResult{PackType: PackTypeEpisode, EpisodeCount: 1}
	unknown := SearchResult{PackType: PackTypeUnknown}

	if !(effectiveEpisodeCount(season) > effectiveEpisodeCount(multi5)) {
		t.Error("season pack must outrank any multi-episode pack")
	}
	if !(effectiveEpisodeCount(multi5) > effectiveEpisodeCount(multi2)) {
		t.Error("bigger multi pack must outrank smaller multi pack")
	}
	if !(effectiveEpisodeCount(multi2) > effectiveEpisodeCount(episode)) {
		t.Error("multi pack must outrank single episode")
	}
	if !(effectiveEpisodeCount(episode) > effectiveEpisodeCount(unknown)) {
		t.Error("classified episode must outrank unknown-classification release")
	}
}

func TestSearchSort_SeasonPackBeatsEpisodeAtSameQuality(t *testing.T) {
	// Given the same quality tier and the same seed bucket, a season pack
	// must outrank an individual episode. This is the minimum guarantee
	// that lets "Interactive Search Season" surface packs at the top.
	results := []SearchResult{
		{
			Release:      plugin.Release{Title: "Breaking.Bad.S01E05.1080p.WEB", Seeds: 50, AgeDays: 30},
			QualityScore: 30610,
			PackType:     PackTypeEpisode,
			EpisodeCount: 1,
		},
		{
			Release:      plugin.Release{Title: "Breaking.Bad.S01.1080p.WEB", Seeds: 50, AgeDays: 30},
			QualityScore: 30610,
			PackType:     PackTypeSeason,
		},
	}

	sortSearchResults(results)

	if results[0].PackType != PackTypeSeason {
		t.Fatalf("season pack must rank first at same quality + seeds; top was %q (%v)",
			results[0].Title, results[0].PackType)
	}
}

func TestSearchSort_QualityStillTrumpsPackType(t *testing.T) {
	// Quality trumps all. A 480p season pack must NOT outrank a 1080p
	// individual episode — Sonarr's "Quality Trumps All" rule.
	results := []SearchResult{
		{
			Release:      plugin.Release{Title: "Breaking.Bad.S01.480p.WEB", Seeds: 500, AgeDays: 30},
			QualityScore: 10610, // 480p tier
			PackType:     PackTypeSeason,
		},
		{
			Release:      plugin.Release{Title: "Breaking.Bad.S01E05.1080p.BluRay", Seeds: 5, AgeDays: 30},
			QualityScore: 30720, // 1080p tier
			PackType:     PackTypeEpisode,
			EpisodeCount: 1,
		},
	}

	sortSearchResults(results)

	if results[0].QualityScore != 30720 {
		t.Fatalf("quality must trump pack type: 1080p episode should outrank 480p season pack, "+
			"got top quality %d", results[0].QualityScore)
	}
}

func TestSearchSort_SeasonPackBeatsMultiEpisodePack(t *testing.T) {
	// A full season pack outranks a multi-episode pack of the same quality.
	results := []SearchResult{
		{
			Release:      plugin.Release{Title: "Show.S01E01-E05.1080p", Seeds: 100, AgeDays: 10},
			QualityScore: 30610,
			PackType:     PackTypeMulti,
			EpisodeCount: 5,
		},
		{
			Release:      plugin.Release{Title: "Show.S01.1080p", Seeds: 20, AgeDays: 10},
			QualityScore: 30610,
			PackType:     PackTypeSeason,
		},
	}

	sortSearchResults(results)

	if results[0].PackType != PackTypeSeason {
		t.Fatalf("season pack must outrank multi-episode pack at same quality; top was %v",
			results[0].PackType)
	}
}
