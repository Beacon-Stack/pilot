package v1

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/show"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/ratelimit"
	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func TestSearchPlan_SeasonIsOneGroupWithFallbackPhrasing(t *testing.T) {
	series := show.Series{Title: "Andor", IMDBID: "tt9253284", TMDBID: 83867, TVDBID: 393189}
	groups := searchPlan(series, 1, 0, 0, false)
	if len(groups) != 1 {
		t.Fatalf("season search: %d groups, want 1 (one request per indexer unless it falls short)", len(groups))
	}
	g := groups[0]
	if len(g.Queries) != 2 || g.Queries[0].Query != "Andor S01" || g.Queries[1].Query != "Andor Season 1" {
		t.Errorf("queries = %+v, want S01 then Season 1", g.Queries)
	}
	if g.Enough == nil {
		t.Error("season group needs an Enough check, or Season 1 never runs where S01 found only episodes")
	}
	for _, q := range g.Queries {
		if q.IMDBID != "tt9253284" || q.TMDBID != 83867 || q.TVDBID != 393189 || q.Season != 1 {
			t.Errorf("query %+v doesn't carry the series IDs and season", q)
		}
	}
}

func TestSearchPlan_EpisodeAndAnimeVariantsStaySeparate(t *testing.T) {
	if got := searchPlan(show.Series{Title: "Andor"}, 1, 5, 0, false); len(got) != 1 || len(got[0].Queries) != 1 {
		t.Errorf("episode plan = %+v, want one single-query group", got)
	}
	// Anime variants find differently named releases of one episode, so
	// they are unioned, not tried as fallbacks.
	if got := searchPlan(show.Series{Title: "Jujutsu Kaisen"}, 1, 48, 48, true); len(got) != 3 {
		t.Errorf("anime episode plan has %d groups, want 3", len(got))
	}
}

func TestSeasonPackFound(t *testing.T) {
	enough := seasonPackFound("Andor", nil, 1)
	pack := func(title string) indexer.SearchResult {
		r := mkResults(title)[0]
		r.PackType = indexer.PackTypeSeason
		return r
	}
	cases := []struct {
		name    string
		results []indexer.SearchResult
		want    bool
	}{
		{"episodes only", mkResults("Andor.S01E01.1080p", "Andor.S01E02.1080p"), false},
		{"pack of this season", []indexer.SearchResult{pack("Andor.S01.1080p")}, true},
		{"pack of another season", []indexer.SearchResult{pack("Andor.S02.1080p")}, false},
		{"pack of another show", []indexer.SearchResult{pack("Anderson.S01.1080p")}, false},
	}
	for _, c := range cases {
		if got := enough(c.results); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// ── Stream endpoint ─────────────────────────────────────────────────────────

type streamTestQuerier struct {
	db.Querier
	rows []db.IndexerConfig
}

func (q streamTestQuerier) ListEnabledIndexers(context.Context) ([]db.IndexerConfig, error) {
	return q.rows, nil
}
func (q streamTestQuerier) ListEnabledReleaseProfiles(context.Context) ([]db.ReleaseProfile, error) {
	return nil, nil
}
func (q streamTestQuerier) GetSetting(context.Context, string) (string, error) {
	return "", sql.ErrNoRows
}
func (q streamTestQuerier) SetSetting(context.Context, db.SetSettingParams) error { return nil }

type gatedIndexer struct {
	titles []string
	gate   chan struct{} // nil: answer at once
}

func (g *gatedIndexer) Name() string               { return "gated" }
func (g *gatedIndexer) Protocol() plugin.Protocol  { return plugin.ProtocolTorrent }
func (g *gatedIndexer) Test(context.Context) error { return nil }
func (g *gatedIndexer) GetRecent(context.Context) ([]plugin.Release, error) {
	return nil, nil
}
func (g *gatedIndexer) Capabilities(context.Context) (plugin.Capabilities, error) {
	return plugin.Capabilities{}, nil
}
func (g *gatedIndexer) Search(ctx context.Context, _ plugin.SearchQuery) ([]plugin.Release, error) {
	if g.gate != nil {
		select {
		case <-g.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	out := make([]plugin.Release, len(g.titles))
	for i, title := range g.titles {
		out[i] = plugin.Release{GUID: title, Title: title, Seeds: 50}
	}
	return out, nil
}

// streamingRecorder hands each flushed chunk to the test as it happens.
type streamingRecorder struct {
	*httptest.ResponseRecorder
	pw *io.PipeWriter
}

func (s *streamingRecorder) Write(p []byte) (int, error) { return s.pw.Write(p) }
func (s *streamingRecorder) Flush()                      {}

func TestStreamReleases_SendsFastResultsFirstThenDone(t *testing.T) {
	release := make(chan struct{})
	indexers := map[string]*gatedIndexer{
		"fast": {titles: []string{"Andor.S01.1080p.WEB-DL"}},
		"slow": {titles: []string{"Andor.S01.2160p.WEB-DL"}, gate: release},
	}
	reg := registry.New()
	var rows []db.IndexerConfig
	for _, id := range []string{"fast", "slow"} {
		idx := indexers[id]
		reg.RegisterIndexer(id, func(json.RawMessage) (plugin.Indexer, error) { return idx, nil })
		rows = append(rows, db.IndexerConfig{ID: id, Name: id, Kind: id, Enabled: true, Settings: "{}"})
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := indexer.NewService(streamTestQuerier{rows: rows}, reg, nil, ratelimit.New(), logger)

	series := show.Series{Title: "Andor"}
	rs := &releaseSearch{
		series:  series,
		season:  1,
		groups:  searchPlan(series, 1, 0, 0, false),
		blocked: map[string]bool{},
	}

	pr, pw := io.Pipe()
	w := &streamingRecorder{ResponseRecorder: httptest.NewRecorder(), pw: pw}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/series/x/releases/stream?season=1", nil)
	hctx := humatest.NewContext(&huma.Operation{}, req, w)
	go func() {
		streamReleases(hctx, svc, rs)
		pw.Close()
	}()

	var lines []releaseListBody
	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		var body releaseListBody
		if err := json.Unmarshal(scanner.Bytes(), &body); err != nil {
			t.Fatalf("line %q is not a release list: %v", scanner.Text(), err)
		}
		lines = append(lines, body)
		// Let the slow indexer answer only once the fast result is out.
		if len(body.Releases) == 1 && !body.Done {
			select {
			case <-release:
			default:
				close(release)
			}
		}
	}

	if len(lines) < 2 {
		t.Fatalf("got %d lines, want partial results then a final line", len(lines))
	}
	sawPartial := false
	for _, l := range lines[:len(lines)-1] {
		if l.Done {
			t.Error("a line before the last says done")
		}
		if len(l.Releases) == 1 && l.Releases[0].Title == "Andor.S01.1080p.WEB-DL" {
			sawPartial = true
		}
	}
	if !sawPartial {
		t.Error("the fast indexer's result never arrived on its own")
	}
	last := lines[len(lines)-1]
	if !last.Done || len(last.Releases) != 2 {
		t.Errorf("last line: done=%v with %d releases, want done with both", last.Done, len(last.Releases))
	}
	if last.Releases[0].Title != "Andor.S01.2160p.WEB-DL" {
		t.Errorf("final order starts with %q, want the 2160p ranked first", last.Releases[0].Title)
	}
	if got := w.Header().Get("Content-Type"); got != "application/x-ndjson" {
		t.Errorf("content type %q", got)
	}
}
