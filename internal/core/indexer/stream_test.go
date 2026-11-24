package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/ratelimit"
	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// streamQuerier answers the queries a search makes, and keeps settings
// in memory. Anything else panics through the nil embedded interface,
// which is what we want.
type streamQuerier struct {
	db.Querier
	rows     []db.IndexerConfig
	settings *settingsStore
}

type settingsStore struct {
	mu sync.Mutex
	m  map[string]string
}

func (q streamQuerier) ListEnabledIndexers(context.Context) ([]db.IndexerConfig, error) {
	return q.rows, nil
}

func (q streamQuerier) ListEnabledReleaseProfiles(context.Context) ([]db.ReleaseProfile, error) {
	return nil, nil
}

func (q streamQuerier) GetSetting(_ context.Context, key string) (string, error) {
	if q.settings == nil {
		return "", sql.ErrNoRows
	}
	q.settings.mu.Lock()
	defer q.settings.mu.Unlock()
	v, ok := q.settings.m[key]
	if !ok {
		return "", sql.ErrNoRows
	}
	return v, nil
}

func (q streamQuerier) SetSetting(_ context.Context, arg db.SetSettingParams) error {
	if q.settings == nil {
		return nil
	}
	q.settings.mu.Lock()
	defer q.settings.mu.Unlock()
	q.settings.m[arg.Key] = arg.Value
	return nil
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

// Struggling indexers

func TestIndexerHealth_WindowNotStreak(t *testing.T) {
	var h indexerHealth
	h.record("x", true)
	if h.struggling("x") {
		t.Fatal("one miss is noise, not struggling")
	}
	h.record("x", true)
	if !h.struggling("x") {
		t.Fatal("two misses in a row: struggling")
	}
	// A cached answer comes back fast; it must not undo two cold misses.
	h.record("x", false)
	if !h.struggling("x") {
		t.Fatal("miss, miss, hit: still struggling (2 of the last 3)")
	}
	h.record("x", false)
	if h.struggling("x") {
		t.Fatal("two hits in the last three: recovered")
	}
	if h.struggling("never-seen") {
		t.Fatal("an indexer with no history isn't struggling")
	}
}

func shortenStruggleTimeout(t *testing.T) {
	t.Helper()
	old := struggleTimeout
	struggleTimeout = 50 * time.Millisecond
	t.Cleanup(func() { struggleTimeout = old })
}

// A person waiting doesn't wait on an indexer that keeps missing: it's
// cut short and reported pending, and everyone else's results are final.
func TestSearchInteractive_StrugglingIndexerCutShort(t *testing.T) {
	shortenStruggleTimeout(t)
	slow := &scriptedIndexer{gate: map[string]chan struct{}{"Show S01": make(chan struct{})}}
	fast := &scriptedIndexer{answers: map[string][]string{"Show S01": {"Show.S01.1080p"}}}
	svc := streamService(t, map[string]*scriptedIndexer{"slow": slow, "fast": fast}, "slow", "fast")
	svc.health.record("slow", true)
	svc.health.record("slow", true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	snap, _ := svc.SearchInteractive(ctx, []SearchGroup{{Queries: queries("Show S01")}}, nil)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("search took %v; the struggling indexer should have been cut at %v", elapsed, struggleTimeout)
	}
	if !snap.Outcomes[0].Pending {
		t.Errorf("struggling indexer outcome %+v, want pending", snap.Outcomes[0])
	}
	if len(snap.Results) != 1 {
		t.Errorf("results = %d, want the fast indexer's one", len(snap.Results))
	}
}

// Automatic searches keep the full wait: nobody re-searches for them.
func TestSearchStream_StrugglingIndexerGetsFullWait(t *testing.T) {
	shortenStruggleTimeout(t)
	release := make(chan struct{})
	slow := &scriptedIndexer{
		answers: map[string][]string{"Show S01": {"Show.S01.1080p"}},
		gate:    map[string]chan struct{}{"Show S01": release},
	}
	svc := streamService(t, map[string]*scriptedIndexer{"slow": slow}, "slow")
	svc.health.record("slow", true)
	svc.health.record("slow", true)

	time.AfterFunc(200*time.Millisecond, func() { close(release) })
	snap, _ := svc.SearchStream(context.Background(), []SearchGroup{{Queries: queries("Show S01")}}, nil)
	if len(snap.Results) != 1 || snap.Outcomes[0].Pending {
		t.Errorf("auto search gave up on a slow indexer: %+v", snap.Outcomes[0])
	}
}

// A search the caller ended isn't held against the indexers.
func TestSearch_CallerCancelDoesNotCountAsMiss(t *testing.T) {
	slow := &scriptedIndexer{gate: map[string]chan struct{}{"Show S01": make(chan struct{})}}
	svc := streamService(t, map[string]*scriptedIndexer{"slow": slow}, "slow")

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		_, _ = svc.SearchInteractive(ctx, []SearchGroup{{Queries: queries("Show S01")}}, nil)
		cancel()
	}
	if svc.health.struggling("slow") {
		t.Error("caller-ended searches were recorded as the indexer's misses")
	}
}

// An indexer waiting out its own rate limit must not hold a concurrency
// slot: with one slot, the other indexer still goes straight away.
func TestSearch_RateLimitWaitHoldsNoSlot(t *testing.T) {
	limited := &scriptedIndexer{answers: map[string][]string{"Show S01": {"Limited.S01"}}}
	free := &scriptedIndexer{answers: map[string][]string{"Show S01": {"Free.S01"}}}
	// "limited" last: Go tends to run the newest goroutine first, so under
	// the old ordering it reliably took the only slot and then waited.
	svc := streamService(t, map[string]*scriptedIndexer{"limited": limited, "free": free}, "free", "limited")
	svc.sem = make(chan struct{}, 1)
	// 60 a minute: the next request is a second away once the token's spent.
	q := svc.q.(streamQuerier)
	for i := range q.rows {
		if q.rows[i].ID == "limited" {
			q.rows[i].Settings = `{"rate_limit": 60}`
		}
	}
	svc.q = q
	if err := svc.rl.Wait(context.Background(), "limited", 60); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	var mu sync.Mutex
	var freeAt time.Duration
	snap, _ := svc.SearchStream(context.Background(), []SearchGroup{{Queries: queries("Show S01")}},
		func(s Snapshot) {
			mu.Lock()
			defer mu.Unlock()
			if freeAt == 0 && titlesOf(s.Results)["Free.S01"] {
				freeAt = time.Since(start)
			}
		})
	if freeAt == 0 || freeAt > 400*time.Millisecond {
		t.Errorf("free indexer answered after %v; it waited behind the rate-limited one's slot", freeAt)
	}
	if got := titlesOf(snap.Results); !got["Limited.S01"] {
		t.Errorf("results %v: the rate-limited indexer should still answer once allowed", got)
	}
}

// Indexer health survives a restart: a new Service loads what the old
// one saved, so a struggling indexer is cut short from the first search.
func TestHealth_SurvivesRestart(t *testing.T) {
	shortenStruggleTimeout(t)
	store := &settingsStore{m: map[string]string{}}
	stuck := &scriptedIndexer{gate: map[string]chan struct{}{"Show S01": make(chan struct{})}}
	reg := registry.New()
	reg.RegisterIndexer("scripted-stuck", func(json.RawMessage) (plugin.Indexer, error) { return stuck, nil })
	q := streamQuerier{rows: []db.IndexerConfig{{ID: "stuck", Name: "stuck", Kind: "scripted-stuck", Enabled: true, Settings: "{}"}}, settings: store}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	before := NewService(q, reg, nil, ratelimit.New(), logger)
	before.health.record("stuck", true)
	before.health.record("stuck", true)
	before.saveHealth()

	after := NewService(q, reg, nil, ratelimit.New(), logger)
	if err := after.LoadHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !after.health.struggling("stuck") {
		t.Fatal("a restarted service forgot the indexer keeps missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	snap, _ := after.SearchInteractive(ctx, []SearchGroup{{Queries: queries("Show S01")}}, nil)
	if time.Since(start) > time.Second || !snap.Outcomes[0].Pending {
		t.Errorf("first search after restart took %v (outcome %+v); want the short cut-off", time.Since(start), snap.Outcomes[0])
	}
}

// Searches save health on their own, in the background.
func TestHealth_SavedAfterSearch(t *testing.T) {
	store := &settingsStore{m: map[string]string{}}
	ok := &scriptedIndexer{answers: map[string][]string{"Show S01": {"Show.S01.1080p"}}}
	svc := streamService(t, map[string]*scriptedIndexer{"ok": ok}, "ok")
	q := svc.q.(streamQuerier)
	q.settings = store
	svc.q = q

	_, _ = svc.SearchStream(context.Background(), []SearchGroup{{Queries: queries("Show S01")}}, nil)
	deadline := time.Now().Add(2 * time.Second)
	for {
		store.mu.Lock()
		v, saved := store.m[healthSettingKey]
		store.mu.Unlock()
		if saved {
			if !strings.Contains(v, `"ok":[false]`) {
				t.Errorf("saved health = %s, want the hit recorded", v)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("health was not saved after a search")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLoadHealth_NothingSavedOrJunk(t *testing.T) {
	store := &settingsStore{m: map[string]string{}}
	svc := NewService(streamQuerier{settings: store}, registry.New(), nil, ratelimit.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.LoadHealth(context.Background()); err != nil {
		t.Errorf("nothing saved should start fresh, got %v", err)
	}
	store.m[healthSettingKey] = "not json"
	if err := svc.LoadHealth(context.Background()); err == nil {
		t.Error("junk should be reported, so the caller can log and start fresh")
	}
}
