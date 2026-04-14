package indexer

import (
	"sort"
	"testing"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

// TestSeedWeight_BucketsAndAgeCap locks down the exact seed-weight buckets so
// accidental regressions of the ranking fix don't silently let inflated seed
// counts dominate again.
func TestSeedWeight_BucketsAndAgeCap(t *testing.T) {
	cases := []struct {
		name    string
		seeds   int
		ageDays float64
		want    int
	}{
		{"zero seeds", 0, 1, 0},
		{"one seed", 1, 1, 0},
		{"three seeds", 3, 1, 0},
		{"four seeds", 4, 1, 1},
		{"ten seeds", 10, 30, 1},
		{"31 seeders", 31, 30, 1},                       // log10(31)=1.49 → 1
		{"32 seeders", 32, 30, 2},                       // log10(32)=1.505 → 2
		{"100 seeders", 100, 30, 2},                     // log10(100)=2.0 → 2
		{"316 seeders", 316, 30, 2},                     // log10(316)=2.4997 → 2
		{"317 seeders", 317, 30, 3},                     // log10(317)=2.5011 → 3
		{"847 seeders new release", 847, 30, 3},         // log10(847)=2.93 → 3
		{"847 seeders old release capped", 847, 500, 1}, // age > 365 → cap to 1
		{"1 seeder old release", 1, 500, 0},             // cap only downgrades; doesn't bump zero
		{"5 seeders old release capped", 5, 500, 1},     // below cap already
		{"5000 seeders old release capped", 5000, 2000, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := seedWeight(tc.seeds, tc.ageDays)
			if got != tc.want {
				t.Errorf("seedWeight(seeds=%d, age=%v) = %d, want %d",
					tc.seeds, tc.ageDays, got, tc.want)
			}
		})
	}
}

// TestSearchSort_ReproducesIncident is the regression test for the exact
// failure: a 5-year-old release with 847 indexer-claimed seeds should NOT
// rank above a fresh release with 20 real seeds of equal quality.
//
// If this test fails, the "dead torrent rises to the top" bug is back.
func TestSearchSort_ReproducesIncident(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Dead.TGx.Release",
				Seeds:   847,
				AgeDays: 1800, // 5 years
			},
			QualityScore: 30610, // 1080p WEB x264 roughly
		},
		{
			Release: plugin.Release{
				Title:   "Fresh.Live.Release",
				Seeds:   20,
				AgeDays: 10,
			},
			QualityScore: 30610, // same quality tier
		},
	}

	sortSearchResults(results)

	if results[0].Title != "Fresh.Live.Release" {
		t.Fatalf("expected Fresh.Live.Release at top (fresh 20 seeds should beat dead 847), "+
			"got: %q. This means the dead-torrent ranking regression is back. Check seedWeight() "+
			"in service.go.", results[0].Title)
	}
}

// TestSearchSort_QualityPrimacyPreserved asserts that a higher-quality release
// still wins even if its seed count is lower — we must not introduce a
// regression where seeds dominate quality.
func TestSearchSort_QualityPrimacyPreserved(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "720p.WEB.x264.Plenty.Seeds",
				Seeds:   500,
				AgeDays: 5,
			},
			QualityScore: 20610, // 720p tier
		},
		{
			Release: plugin.Release{
				Title:   "1080p.Bluray.x265.Few.Seeds",
				Seeds:   5,
				AgeDays: 5,
			},
			QualityScore: 30720, // 1080p Bluray tier (higher)
		},
	}

	sortSearchResults(results)

	if results[0].Title != "1080p.Bluray.x265.Few.Seeds" {
		t.Fatalf("quality primacy broken: 1080p Bluray should outrank 720p WEB "+
			"regardless of seed count. got top: %q", results[0].Title)
	}
}

// TestSearchSort_SameQualityMoreSeedsWins verifies that within the same quality
// tier and same age bucket, more seeds still wins (the tiebreaker still works
// for legitimate cases).
func TestSearchSort_SameQualityMoreSeedsWins(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Low.Seeds.Fresh",
				Seeds:   8, // bucket 1
				AgeDays: 5,
			},
			QualityScore: 30610,
		},
		{
			Release: plugin.Release{
				Title:   "High.Seeds.Fresh",
				Seeds:   150, // bucket 2
				AgeDays: 5,
			},
			QualityScore: 30610,
		},
	}

	sortSearchResults(results)

	if results[0].Title != "High.Seeds.Fresh" {
		t.Fatalf("same-quality same-age tiebreaker broken: more seeds should win. got top: %q", results[0].Title)
	}
}

// TestSearchSort_AgeTiebreakerOnTies verifies the final tiebreaker: when two
// releases are identical in quality AND seed bucket, the newer one wins.
func TestSearchSort_AgeTiebreakerOnTies(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Old.Same.Bucket",
				Seeds:   12, // bucket 1
				AgeDays: 100,
			},
			QualityScore: 30610,
		},
		{
			Release: plugin.Release{
				Title:   "New.Same.Bucket",
				Seeds:   12, // bucket 1
				AgeDays: 5,
			},
			QualityScore: 30610,
		},
	}

	sortSearchResults(results)

	if results[0].Title != "New.Same.Bucket" {
		t.Fatalf("age tiebreaker broken: newer release should win on tied quality+seed bucket. got top: %q", results[0].Title)
	}
}

// ── Filter pass tests ────────────────────────────────────────────────────────
//
// These exercise applyMinSeedersFilter() directly. The filter is the
// primary safety net against dead/inflated torrents in Pilot's search
// pipeline. Regressions here mean the UI starts grabbing releases that
// should have been flagged.

// TestFilterPass_BelowMinSeedersGetsTagged is the headline case: a release
// with seeds below the per-indexer threshold gets a FilterReason entry
// that the UI uses to render the row grayed with an override button.
func TestFilterPass_BelowMinSeedersGetsTagged(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Low.Seeds.Release",
				Seeds:   2,
				AgeDays: 30,
			},
			IndexerID:    "idx-1",
			QualityScore: 30000,
		},
	}
	applyMinSeedersFilter(results, map[string]int{"idx-1": 5})

	if len(results[0].FilterReasons) != 1 {
		t.Fatalf("expected 1 filter reason, got %d", len(results[0].FilterReasons))
	}
	want := "below minimum seeders (2 < 5)"
	if results[0].FilterReasons[0] != want {
		t.Errorf("wrong filter reason: got %q want %q", results[0].FilterReasons[0], want)
	}
}

// TestFilterPass_AboveMinSeedersUntagged verifies the happy path: releases
// above the threshold pass through untouched.
func TestFilterPass_AboveMinSeedersUntagged(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Healthy.Release",
				Seeds:   50,
				AgeDays: 10,
			},
			IndexerID:    "idx-1",
			QualityScore: 30000,
		},
	}
	applyMinSeedersFilter(results, map[string]int{"idx-1": 5})

	if len(results[0].FilterReasons) != 0 {
		t.Errorf("healthy release should have no filter reasons, got %v", results[0].FilterReasons)
	}
}

// TestFilterPass_FreshMultiIndexerBypass is the critical tracker-aggregation-
// lag exception: a brand-new release (<12h) confirmed on 2+ indexers gets
// to bypass the min_seeders filter even with 0 seeders, because public
// trackers lag behind the real peer state on hot new releases.
//
// Removing this exception would cause Pilot to wrongly reject legitimate
// day-one torrents until the indexer catches up — a UX regression.
func TestFilterPass_FreshMultiIndexerBypass(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Brand.New.Hot.Release",
				Seeds:   0,   // "zero seeders" but we trust it because two indexers agree
				AgeDays: 0.2, // ~5 hours
			},
			IndexerID:    "idx-1",
			QualityScore: 30000,
		},
		{
			Release: plugin.Release{
				Title:   "Brand.New.Hot.Release", // same title — multi_indexer trigger
				Seeds:   0,
				AgeDays: 0.2,
			},
			IndexerID:    "idx-2",
			QualityScore: 30000,
		},
	}
	applyMinSeedersFilter(results, map[string]int{"idx-1": 5, "idx-2": 5})

	for i, r := range results {
		if len(r.FilterReasons) != 0 {
			t.Errorf("result %d should pass freshness exception, got filter reasons: %v", i, r.FilterReasons)
		}
	}
}

// TestFilterPass_OldReleaseNoBypass verifies the freshness exception does
// NOT apply to old releases even when multi-indexer — the 847-seeders-on-
// a-5-year-old-release incident. Old + multi-indexer is actually the most
// common fake-seed-count shape (aggregators crosslink stale data).
func TestFilterPass_OldReleaseNoBypass(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Old.TGx.Release",
				Seeds:   2,
				AgeDays: 1800, // 5 years
			},
			IndexerID:    "idx-1",
			QualityScore: 30000,
		},
		{
			Release: plugin.Release{
				Title:   "Old.TGx.Release",
				Seeds:   2,
				AgeDays: 1800,
			},
			IndexerID:    "idx-2",
			QualityScore: 30000,
		},
	}
	applyMinSeedersFilter(results, map[string]int{"idx-1": 5, "idx-2": 5})

	for i, r := range results {
		if len(r.FilterReasons) == 0 {
			t.Errorf("result %d: old multi-indexer release should still be filtered, got none", i)
		}
	}
}

// TestFilterPass_SingleIndexerFreshNoBypass verifies the freshness exception
// requires 2+ indexers — a single-indexer fresh release with 0 seeders is
// still filtered, because we can't trust a lone indexer's freshness claim.
func TestFilterPass_SingleIndexerFreshNoBypass(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Single.Indexer.Release",
				Seeds:   0,
				AgeDays: 0.1,
			},
			IndexerID:    "idx-1",
			QualityScore: 30000,
		},
	}
	applyMinSeedersFilter(results, map[string]int{"idx-1": 5})

	if len(results[0].FilterReasons) == 0 {
		t.Error("single-indexer fresh release with 0 seeds should still be filtered")
	}
}

// TestFilterPass_DefaultMinSeeders verifies that a missing/zero per-indexer
// threshold falls back to 5. This matters for edge cases where an indexer
// exists in search results but not in the configs list (impossible in
// production, but tests should not crash).
func TestFilterPass_DefaultMinSeeders(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Default.Fallback",
				Seeds:   3, // below default 5
				AgeDays: 30,
			},
			IndexerID:    "unknown-indexer",
			QualityScore: 30000,
		},
	}
	applyMinSeedersFilter(results, nil) // empty map

	if len(results[0].FilterReasons) != 1 {
		t.Errorf("expected default min_seeders=5 to filter a 3-seed release, got: %v",
			results[0].FilterReasons)
	}
}

// TestFilterPass_PerIndexerDifferentThresholds verifies that the same seed
// count can pass on one indexer (min=1) and fail on another (min=20).
// This is how users tune per-indexer strictness — low for private trackers,
// high for flaky public ones.
func TestFilterPass_PerIndexerDifferentThresholds(t *testing.T) {
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Private.Tracker.Release",
				Seeds:   2,
				AgeDays: 30,
			},
			IndexerID:    "private-idx",
			QualityScore: 30000,
		},
		{
			Release: plugin.Release{
				Title:   "Public.Tracker.Release",
				Seeds:   15,
				AgeDays: 30,
			},
			IndexerID:    "public-idx",
			QualityScore: 30000,
		},
	}
	applyMinSeedersFilter(results, map[string]int{
		"private-idx": 1,  // lax — private trackers enforce ratios
		"public-idx":  20, // strict — TRaSH recommendation
	})

	if len(results[0].FilterReasons) != 0 {
		t.Errorf("private-tracker 2-seed release should pass min=1, got: %v", results[0].FilterReasons)
	}
	if len(results[1].FilterReasons) != 1 {
		t.Errorf("public-tracker 15-seed release should fail min=20, got: %v", results[1].FilterReasons)
	}
}

// TestFilterPass_ReproducesOriginalIncident end-to-ends the ORIGINAL bug:
// a 5-year-old release with 847 seeders, one indexer, no other indexers
// confirming it. It must be ranked below any alternative AND flagged
// with a filter reason under a reasonable min_seeders.
//
// Wait — 847 > 5, so the min_seeders filter alone wouldn't have caught
// this. The incident was caught by the ranking layer (seedWeight log10
// + age cap), not the filter pass. This test documents that fact:
// min_seeders alone is insufficient. The ranking fix is load-bearing.
func TestFilterPass_DoesNotCatchInflatedSeeders(t *testing.T) {
	// This test intentionally asserts what the filter pass does NOT do:
	// it's a contract pin that the filter is not the sole defense against
	// the original incident. Future-you should remember that ranking +
	// stall detection are the other two legs of the stool.
	results := []SearchResult{
		{
			Release: plugin.Release{
				Title:   "Raised.by.Wolves.2020.S01E01.WEB.x264-PHOENiX[TGx]",
				Seeds:   847,  // the fake claim
				AgeDays: 1800, // 5 years
			},
			IndexerID:    "1337x",
			QualityScore: 30000,
		},
	}
	applyMinSeedersFilter(results, map[string]int{"1337x": 5})

	// The filter does nothing — 847 > 5.
	if len(results[0].FilterReasons) != 0 {
		t.Errorf("filter pass should NOT catch 847 inflated seeders on its own. "+
			"If this test is now failing, you've tightened the filter — make sure "+
			"you haven't also forgotten to relax TestFilterPass_FreshMultiIndexerBypass. "+
			"Got reasons: %v", results[0].FilterReasons)
	}

	// Verify that the RANKING layer (sortSearchResults) does catch it, given
	// a fresh alternative of equal quality.
	resultsWithAlternative := []SearchResult{
		results[0],
		{
			Release: plugin.Release{
				Title:   "Raised.by.Wolves.2020.S01E01.1080p.WEB.H264-CAKES",
				Seeds:   20,
				AgeDays: 10,
			},
			IndexerID:    "1337x",
			QualityScore: 30000,
		},
	}
	sortSearchResults(resultsWithAlternative)
	if resultsWithAlternative[0].Title != "Raised.by.Wolves.2020.S01E01.1080p.WEB.H264-CAKES" {
		t.Fatal("ranking layer failed to put the fresh alternative above the old 847-seed fake. " +
			"The dead torrent regression is back — check seedWeight() + sort in service.go.")
	}
}

// sortSearchResults mirrors the sort logic in Search() so tests don't have to
// stand up a full service. Keep this in lockstep with the real sort closure
// in Search() — if they diverge, the tests are lying.
func sortSearchResults(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		si, sj := results[i].QualityScore, results[j].QualityScore
		if si != sj {
			return si > sj
		}
		ei, ej := effectiveEpisodeCount(results[i]), effectiveEpisodeCount(results[j])
		if ei != ej {
			return ei > ej
		}
		wi := seedWeight(results[i].Seeds, results[i].AgeDays)
		wj := seedWeight(results[j].Seeds, results[j].AgeDays)
		if wi != wj {
			return wi > wj
		}
		return results[i].AgeDays < results[j].AgeDays
	})
}
