package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/ratelimit"
	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// streamQuerier answers the three queries a search makes. Anything else
// panics through the nil embedded interface, which is what we want.
type streamQuerier struct {
	db.Querier
	rows []db.IndexerConfig
}

func (q streamQuerier) ListEnabledIndexers(context.Context) ([]db.IndexerConfig, error) {
	return q.rows, nil
}

func (q streamQuerier) ListEnabledReleaseProfiles(context.Context) ([]db.ReleaseProfile, error) {
	return nil, nil
}

func (q streamQuerier) GetSetting(context.Context, string) (string, error) {
	return "", sql.ErrNoRows
}

// scriptedIndexer answers each query text with fixed titles. A query in
// gate waits until that channel is closed (or the context ends).
type scriptedIndexer struct {
	mu      sync.Mutex
	answers map[string][]string
	errs    map[string]error
	gate    map[string]chan struct{}
	asked   []string
}

func (f *scriptedIndexer) Name() string               { return "scripted" }
func (f *scriptedIndexer) Protocol() plugin.Protocol  { return plugin.ProtocolTorrent }
func (f *scriptedIndexer) Test(context.Context) error { return nil }
func (f *scriptedIndexer) GetRecent(context.Context) ([]plugin.Release, error) {
	return nil, nil
}
func (f *scriptedIndexer) Capabilities(context.Context) (plugin.Capabilities, error) {
	return plugin.Capabilities{}, nil
}

func (f *scriptedIndexer) Search(ctx context.Context, q plugin.SearchQuery) ([]plugin.Release, error) {
	f.mu.Lock()
	f.asked = append(f.asked, q.Query)
	gate := f.gate[q.Query]
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := f.errs[q.Query]; err != nil {
		return nil, err
	}
	var out []plugin.Release
	for _, title := range f.answers[q.Query] {
		out = append(out, plugin.Release{GUID: title, Title: title})
	}
	return out, nil
}

func (f *scriptedIndexer) queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

// streamService wires a Service to scripted indexers, keyed by indexer ID.
func streamService(t *testing.T, indexers map[string]*scriptedIndexer, order ...string) *Service {
	t.Helper()
	reg := registry.New()
	var rows []db.IndexerConfig
	for _, id := range order {
		idx := indexers[id]
		kind := "scripted-" + id
		reg.RegisterIndexer(kind, func(json.RawMessage) (plugin.Indexer, error) { return idx, nil })
		rows = append(rows, db.IndexerConfig{ID: id, Name: id, Kind: kind, Enabled: true, Settings: "{}"})
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(streamQuerier{rows: rows}, reg, nil, ratelimit.New(), logger)
}

func queries(texts ...string) []plugin.SearchQuery {
	out := make([]plugin.SearchQuery, len(texts))
	for i, t := range texts {
		out[i] = plugin.SearchQuery{Query: t, Season: 1}
	}
	return out
}

func titlesOf(rs []SearchResult) map[string]bool {
	m := map[string]bool{}
	for _, r := range rs {
		m[r.Title] = true
	}
	return m
}

// The second phrasing runs only on an indexer whose first answer wasn't
// enough — the point of a group.
func TestSearchStream_FallbackQueryOnlyWhereFirstFallsShort(t *testing.T) {
	packs := &scriptedIndexer{answers: map[string][]string{
		"Show S01":      {"Show.S01.1080p"},
		"Show Season 1": {"Show.Season.1.720p"},
	}}
	episodesOnly := &scriptedIndexer{answers: map[string][]string{
		"Show S01":      {"Show.S01E01.1080p"},
		"Show Season 1": {"Show.Season.1.Complete.1080p"},
	}}
	svc := streamService(t, map[string]*scriptedIndexer{"packs": packs, "eps": episodesOnly}, "packs", "eps")

	hasPack := func(rs []SearchResult) bool {
		for _, r := range rs {
			if r.PackType == PackTypeSeason {
				return true
			}
		}
		return false
	}
	snap, err := svc.SearchStream(context.Background(),
		[]SearchGroup{{Queries: queries("Show S01", "Show Season 1"), Enough: hasPack}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := packs.queries(); len(got) != 1 {
		t.Errorf("indexer with a pack was asked %v, want only the first query", got)
	}
	if got := episodesOnly.queries(); len(got) != 2 {
		t.Errorf("indexer without a pack was asked %v, want both queries", got)
	}
	got := titlesOf(snap.Results)
	for _, want := range []string{"Show.S01.1080p", "Show.S01E01.1080p", "Show.Season.1.Complete.1080p"} {
		if !got[want] {
			t.Errorf("missing %q from %v", want, got)
		}
	}
	if !snap.Done {
		t.Error("final snapshot not marked done")
	}
}

// Without Enough, any result stops the chain and nothing stops it early.
func TestSearchStream_DefaultEnoughIsAnyResult(t *testing.T) {
	empty := &scriptedIndexer{answers: map[string][]string{"B": {"Show.S01E02"}}}
	svc := streamService(t, map[string]*scriptedIndexer{"a": empty}, "a")

	snap, _ := svc.SearchStream(context.Background(), []SearchGroup{{Queries: queries("A", "B", "C")}}, nil)
	if got := empty.queries(); len(got) != 2 || got[1] != "B" {
		t.Errorf("asked %v, want A then B (stop at the first result)", got)
	}
	if len(snap.Results) != 1 {
		t.Errorf("results = %d, want 1", len(snap.Results))
	}
}

// A fast indexer's results reach the caller while a slow one is still
// working — what makes interactive search feel quick.
func TestSearchStream_UpdatesBeforeSlowIndexerAnswers(t *testing.T) {
	release := make(chan struct{})
	fast := &scriptedIndexer{answers: map[string][]string{"Show S01": {"Fast.S01.1080p"}}}
	slow := &scriptedIndexer{
		answers: map[string][]string{"Show S01": {"Slow.S01.1080p"}},
		gate:    map[string]chan struct{}{"Show S01": release},
	}
	svc := streamService(t, map[string]*scriptedIndexer{"fast": fast, "slow": slow}, "fast", "slow")

	var (
		mu      sync.Mutex
		updates []Snapshot
	)
	sawFast := make(chan struct{})
	var once sync.Once
	go func() {
		<-sawFast
		close(release)
	}()
	snap, err := svc.SearchStream(context.Background(), []SearchGroup{{Queries: queries("Show S01")}},
		func(s Snapshot) {
			mu.Lock()
			updates = append(updates, s)
			mu.Unlock()
			if titlesOf(s.Results)["Fast.S01.1080p"] {
				once.Do(func() { close(sawFast) })
			}
		})
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(updates) < 2 {
		t.Fatalf("got %d updates, want the opening one plus one per indexer", len(updates))
	}
	first := updates[0]
	if len(first.Results) != 0 || len(first.Outcomes) != 2 || !first.Outcomes[0].Searching || !first.Outcomes[1].Searching {
		t.Errorf("opening update = %+v, want no results and both indexers searching", first)
	}
	var partial *Snapshot
	for i := range updates {
		if titlesOf(updates[i].Results)["Fast.S01.1080p"] {
			partial = &updates[i]
			break
		}
	}
	if partial == nil {
		t.Fatal("no update carried the fast indexer's result")
	}
	if titlesOf(partial.Results)["Slow.S01.1080p"] || !partial.Outcomes[1].Searching || partial.Done {
		t.Errorf("partial update = %+v, want only the fast result with slow still searching", partial)
	}
	if got := titlesOf(snap.Results); !got["Fast.S01.1080p"] || !got["Slow.S01.1080p"] {
		t.Errorf("final results %v, want both", got)
	}
	for _, o := range snap.Outcomes {
		if o.Searching || o.Results != 1 {
			t.Errorf("final outcome %+v, want finished with 1 result", o)
		}
	}
}

func TestSearchStream_ErrorsAndTimeoutsInOutcomes(t *testing.T) {
	broken := &scriptedIndexer{errs: map[string]error{"Show S01": errors.New("indexer reported error 100")}}
	stuck := &scriptedIndexer{gate: map[string]chan struct{}{"Show S01": make(chan struct{})}}
	svc := streamService(t, map[string]*scriptedIndexer{"broken": broken, "stuck": stuck}, "broken", "stuck")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	snap, err := svc.SearchStream(ctx, []SearchGroup{{Queries: queries("Show S01")}}, nil)
	if err == nil {
		t.Error("want a combined error for the broken indexer")
	}
	byID := map[string]Outcome{}
	for _, o := range snap.Outcomes {
		byID[o.IndexerID] = o
	}
	if byID["broken"].Error == "" || byID["broken"].Pending {
		t.Errorf("broken outcome = %+v, want an error", byID["broken"])
	}
	if !byID["stuck"].Pending || byID["stuck"].Error != "" {
		t.Errorf("stuck outcome = %+v, want pending (still scraping), not failed", byID["stuck"])
	}
}

// Snapshots are tagged afresh each time: a filter reason must not pile
// up once per update.
func TestSearchStream_SnapshotsDoNotAccumulateTags(t *testing.T) {
	a := &scriptedIndexer{answers: map[string][]string{"Show S01": {"Show.S01.1080p"}}}
	b := &scriptedIndexer{answers: map[string][]string{"Show S01": {"Other.S01.1080p"}}}
	svc := streamService(t, map[string]*scriptedIndexer{"a": a, "b": b}, "a", "b")

	snap, _ := svc.SearchStream(context.Background(), []SearchGroup{{Queries: queries("Show S01")}},
		func(Snapshot) {})
	for _, r := range snap.Results {
		// No seeders, so min-seeders tags each release exactly once.
		if len(r.FilterReasons) != 1 {
			t.Errorf("%s has reasons %v, want exactly one", r.Title, r.FilterReasons)
		}
	}
}

// A timed-out query ends that indexer's chain: the next phrasing would
// start a second slow scrape with no time left to finish it.
func TestSearchStream_TimeoutSkipsFallbackQuery(t *testing.T) {
	stuck := &scriptedIndexer{gate: map[string]chan struct{}{"Show S01": make(chan struct{})}}
	svc := streamService(t, map[string]*scriptedIndexer{"stuck": stuck}, "stuck")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	snap, _ := svc.SearchStream(ctx, []SearchGroup{{Queries: queries("Show S01", "Show Season 1")}}, nil)
	if got := stuck.queries(); len(got) != 1 {
		t.Errorf("asked %v after a timeout, want only the first query", got)
	}
	if !snap.Outcomes[0].Pending {
		t.Errorf("outcome %+v, want pending", snap.Outcomes[0])
	}
}
