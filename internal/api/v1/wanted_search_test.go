package v1

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/parser"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func result(title, guid string, seeds int) indexer.SearchResult {
	q := parser.Parse(title).Quality()
	return indexer.SearchResult{
		Release:      plugin.Release{Title: title, GUID: guid, Seeds: seeds, AgeDays: 1, Quality: q},
		QualityScore: q.Score(),
	}
}

// ranked puts results in ranking order, as a search returns them.
func ranked(results ...indexer.SearchResult) []indexer.SearchResult {
	sort.SliceStable(results, func(i, j int) bool { return indexer.RanksAbove(results[i], results[j]) })
	return results
}

// Regression: the "listed on several indexers" preference looked across
// all qualities, so a 720p on two indexers beat a 2160p on one.
func TestPickBestViable_MultiIndexerPreferenceStaysInTopQuality(t *testing.T) {
	viable := ranked(
		result("Show.S01E05.2160p.WEB-DL.x265-GRP", "uhd", 10),
		result("Show.S01E05.720p.WEB-DL.x264-GRP", "hd-1", 10),
		result("Show.S01E05.720p.WEB-DL.x264-GRP", "hd-2", 10),
	)
	if got := pickBestViable(viable); got.GUID != "uhd" {
		t.Fatalf("picked %q (%s), want the 2160p", got.Title, got.GUID)
	}
}

func TestPickBestViable_PrefersMultiIndexerWithinTopQuality(t *testing.T) {
	viable := ranked(
		result("Show.S01E05.1080p.WEB-DL.x264-SOLO", "solo", 50),
		result("Show.S01E05.1080p.WEB-DL.x264-DUO", "duo-1", 50),
		result("Show.S01E05.1080p.WEB-DL.x264-DUO", "duo-2", 50),
	)
	if got := pickBestViable(viable); got.Title != "Show.S01E05.1080p.WEB-DL.x264-DUO" {
		t.Fatalf("picked %q, want the release listed on two indexers", got.Title)
	}
}

// ── Bulk search runner ───────────────────────────────────────────────────────

func TestWantedSearchRunner_OneRunPerKind(t *testing.T) {
	gate := make(chan struct{})
	var mu sync.Mutex
	var searched []string
	done := make(chan string, 4)

	r := newWantedSearchRunner(func(_ context.Context, p autoSearchParams) (*autoSearchResultBody, error) {
		<-gate
		mu.Lock()
		searched = append(searched, p.EpisodeID)
		mu.Unlock()
		return &autoSearchResultBody{Result: "grabbed"}, nil
	}, slog.New(slog.DiscardHandler))
	r.done = func(kind string) { done <- kind }

	targets := []autoSearchParams{{EpisodeID: "e1"}, {EpisodeID: "e2"}}
	if !r.start("missing", targets) {
		t.Fatal("first start refused")
	}
	if r.start("missing", targets) {
		t.Fatal("second concurrent start of the same kind was accepted")
	}
	if !r.start("cutoff", nil) {
		t.Fatal("a different kind should run independently")
	}
	close(gate)

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runs did not finish")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(searched) != 2 || searched[0] != "e1" || searched[1] != "e2" {
		t.Fatalf("searched = %v, want [e1 e2] in order", searched)
	}
	if !r.start("missing", targets) {
		t.Fatal("a finished kind should be startable again")
	}
}

func TestCapTargets(t *testing.T) {
	ts := make([]autoSearchParams, maxBulkSearch+10)
	if got := len(capTargets(ts)); got != maxBulkSearch {
		t.Fatalf("capTargets = %d, want %d", got, maxBulkSearch)
	}
}
