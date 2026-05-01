package importer

// Importer tests live here because the integration that turns
// "[SubsPlease] Jujutsu Kaisen - 48 [1080p].mkv" into TMDB S01E48
// kept silently breaking in production. The parser knew the file
// was absolute episode 48; the animelist service knew abs 48 = S01E48
// for JJK; but the importer wired neither together. Pin the
// resolveEpisodeInfo contract here so future refactors of either side
// can't recreate the gap.

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// stubResolver is a minimal AbsoluteEpisodeResolver for tests. The
// real implementation is *animelist.Service which loads a 50k-entry
// XML; the importer only cares about the boolean + the (season, ep)
// it returns.
type stubResolver struct {
	tmdbID    int
	abs       int
	gotSeason int
	gotEp     int
	gotOK     bool
	calls     int
}

func (s *stubResolver) AbsoluteToTMDBEpisode(tmdbID, abs int) (int, int, bool) {
	s.calls++
	if tmdbID != s.tmdbID || abs != s.abs {
		return 0, 0, false
	}
	return s.gotSeason, s.gotEp, s.gotOK
}

// stubQuerier embeds db.Querier so we only have to override GetSeries.
// Other methods panic if accidentally called — that's loud-by-design;
// the test surface is small and any new dependency surfaces fast.
type stubQuerier struct {
	db.Querier
	series db.Series
	err    error
}

func (q *stubQuerier) GetSeries(_ context.Context, _ string) (db.Series, error) {
	if q.err != nil {
		return db.Series{}, q.err
	}
	return q.series, nil
}

func newSvc(q db.Querier, r AbsoluteEpisodeResolver) *Service {
	return &Service{
		q:      q,
		logger: slog.New(slog.DiscardHandler),
		animeR: r,
	}
}

// ── resolveEpisodeInfo ──────────────────────────────────────────────────────

// Headline regression. Without the anime fallback, this test fails
// and the JJK production bug is back. With it, abs=48 → S01E48.
func TestResolveEpisodeInfo_AnimeAbsoluteResolvesToTMDB(t *testing.T) {
	q := &stubQuerier{series: db.Series{ID: "show-1", TmdbID: 95479}}
	r := &stubResolver{tmdbID: 95479, abs: 48, gotSeason: 1, gotEp: 48, gotOK: true}
	svc := newSvc(q, r)

	info := svc.resolveEpisodeInfo(
		context.Background(),
		"/downloads/[SubsPlease] Jujutsu Kaisen - 48 [1080p].mkv",
		db.GrabHistory{SeriesID: "show-1"},
	)

	if info.Season != 1 {
		t.Errorf("season: got %d, want 1", info.Season)
	}
	if len(info.Episodes) != 1 || info.Episodes[0] != 48 {
		t.Errorf("episodes: got %v, want [48]", info.Episodes)
	}
	if r.calls != 1 {
		t.Errorf("resolver calls: got %d, want 1", r.calls)
	}
}

// SxxExx-style filenames take the parser's natural output and skip
// the resolver entirely. The fallback is for fansub releases only.
func TestResolveEpisodeInfo_SxxExxSkipsResolver(t *testing.T) {
	q := &stubQuerier{series: db.Series{ID: "show-1", TmdbID: 95479}}
	r := &stubResolver{}
	svc := newSvc(q, r)

	info := svc.resolveEpisodeInfo(
		context.Background(),
		"/downloads/Jujutsu.Kaisen.S03E01.1080p.WEB-DL.mkv",
		db.GrabHistory{SeriesID: "show-1"},
	)

	if info.Season != 3 || len(info.Episodes) != 1 || info.Episodes[0] != 1 {
		t.Errorf("expected S03E01 from filename; got S%dE%v", info.Season, info.Episodes)
	}
	if r.calls != 0 {
		t.Errorf("resolver should not be called when SxxExx parses cleanly; got %d calls", r.calls)
	}
}

// Non-anime show: GetSeries returns a series with a TMDB ID, but the
// resolver returns ok=false (no animelist mapping for that show). The
// importer should leave Episodes empty — the downstream "could not
// parse" warning is the right outcome.
func TestResolveEpisodeInfo_NonAnimeAbsoluteStaysEmpty(t *testing.T) {
	q := &stubQuerier{series: db.Series{ID: "show-1", TmdbID: 12345}}
	r := &stubResolver{tmdbID: 12345, abs: 5, gotOK: false}
	svc := newSvc(q, r)

	info := svc.resolveEpisodeInfo(
		context.Background(),
		"/downloads/Some.Show.-.5.1080p.mkv",
		db.GrabHistory{SeriesID: "show-1"},
	)

	if len(info.Episodes) != 0 {
		t.Errorf("non-anime absolute should leave Episodes empty; got %v", info.Episodes)
	}
}

// nil resolver: importer was constructed without animelist support
// (back-compat with the pre-anime construction). Must not panic and
// must not magically populate Episodes.
func TestResolveEpisodeInfo_NilResolverIsNoOp(t *testing.T) {
	q := &stubQuerier{series: db.Series{ID: "show-1", TmdbID: 95479}}
	svc := newSvc(q, nil) // <-- nil resolver

	info := svc.resolveEpisodeInfo(
		context.Background(),
		"/downloads/[SubsPlease] Jujutsu Kaisen - 48 [1080p].mkv",
		db.GrabHistory{SeriesID: "show-1"},
	)

	if len(info.Episodes) != 0 {
		t.Errorf("nil resolver should leave Episodes empty; got %v", info.Episodes)
	}
	if info.AbsoluteEpisode != 48 {
		t.Errorf("absolute should still be parsed; got %d, want 48", info.AbsoluteEpisode)
	}
}

// GetSeries failure: the lookup is best-effort. If we can't find the
// series (deleted, DB hiccup, etc.) the importer should skip the
// resolver fallback rather than crashing.
func TestResolveEpisodeInfo_GetSeriesErrorSkipsResolver(t *testing.T) {
	q := &stubQuerier{err: errors.New("series not found")}
	r := &stubResolver{}
	svc := newSvc(q, r)

	info := svc.resolveEpisodeInfo(
		context.Background(),
		"/downloads/[SubsPlease] Jujutsu Kaisen - 48 [1080p].mkv",
		db.GrabHistory{SeriesID: "missing"},
	)

	if len(info.Episodes) != 0 {
		t.Errorf("GetSeries error should skip fallback; got Episodes %v", info.Episodes)
	}
	if r.calls != 0 {
		t.Errorf("resolver should not be called when GetSeries fails; got %d calls", r.calls)
	}
}

// Filename has neither SxxExx nor an absolute number. Both code paths
// no-op; the file gets skipped downstream with the "could not parse"
// warning. Pin that resolveEpisodeInfo doesn't fabricate an episode.
func TestResolveEpisodeInfo_NeitherSxxExxNorAbsoluteIsNoOp(t *testing.T) {
	q := &stubQuerier{series: db.Series{ID: "show-1", TmdbID: 95479}}
	r := &stubResolver{}
	svc := newSvc(q, r)

	info := svc.resolveEpisodeInfo(
		context.Background(),
		"/downloads/random-blob-no-episode.mkv",
		db.GrabHistory{SeriesID: "show-1"},
	)

	if len(info.Episodes) != 0 {
		t.Errorf("expected empty Episodes; got %v", info.Episodes)
	}
	if r.calls != 0 {
		t.Errorf("resolver should not be called when AbsoluteEpisode=0; got %d calls", r.calls)
	}
}
