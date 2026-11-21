package show

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/beacon-stack/pilot/internal/config"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
)

// idMeta is stubMeta plus the optional external-id lookup.
type idMeta struct {
	stubMeta
	ids   map[int]tmdbtv.ExternalIDs
	asked []int
}

func (m *idMeta) GetExternalIDs(_ context.Context, tmdbID int) (tmdbtv.ExternalIDs, error) {
	m.asked = append(m.asked, tmdbID)
	return m.ids[tmdbID], nil
}

func TestBackfillExternalIDs_RealDB(t *testing.T) {
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	conn, err := d.SQL.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	insert := func(id string, tmdb int, imdb any, tvdb any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, `INSERT INTO series
			(id, tmdb_id, imdb_id, tvdb_id, title, sort_title, year, library_id, quality_profile_id, added_at, updated_at)
			VALUES (?, ?, ?, ?, 't', 't', 2020, 'lib', 'qp', 'now', 'now')`, id, tmdb, imdb, tvdb); err != nil {
			t.Fatal(err)
		}
	}
	insert("andor", 83867, nil, nil)         // added before ids were stored
	insert("obscure", 99999, nil, nil)       // TMDB has no ids for it
	insert("done", 1396, "tt0903747", 81189) // already has both
	conn.Close()

	meta := &idMeta{ids: map[int]tmdbtv.ExternalIDs{83867: {IMDBID: "tt9253284", TVDBID: 393189}}}
	svc := NewService(db.New(d.SQL), meta, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	n, err := svc.BackfillExternalIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || len(meta.asked) != 2 {
		t.Errorf("updated %d, asked %v; want the two series without ids", n, meta.asked)
	}
	got, err := svc.Get(ctx, "andor")
	if err != nil {
		t.Fatal(err)
	}
	if got.TVDBID != 393189 || got.IMDBID != "tt9253284" {
		t.Errorf("andor ids = %d / %q", got.TVDBID, got.IMDBID)
	}

	// Asked once: a second run finds nothing to do, including the series
	// TMDB has no ids for.
	meta.asked = nil
	if n, _ := svc.BackfillExternalIDs(ctx); n != 0 || len(meta.asked) != 0 {
		t.Errorf("second run updated %d, asked %v; want nothing", n, meta.asked)
	}
}

func TestBackfillExternalIDs_ProviderWithoutLookup(t *testing.T) {
	svc := NewService(nil, &stubMeta{}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if n, err := svc.BackfillExternalIDs(context.Background()); n != 0 || err != nil {
		t.Errorf("got %d, %v; want a quiet no-op", n, err)
	}
}

func TestKeepIDsOnRefresh(t *testing.T) {
	stored := int64(393189)
	if got := keepTVDB(&stored, 0); got == nil || *got != 393189 {
		t.Errorf("TMDB briefly missing the id must not wipe it: %v", got)
	}
	if got := keepTVDB(&stored, 400000); *got != 400000 {
		t.Errorf("a fresh id wins: %v", *got)
	}
	if got := keepTVDB(nil, 0); got == nil || *got != 0 {
		t.Errorf("never looked up and TMDB has none: store 0, got %v", got)
	}
	imdb := "tt9253284"
	if got := keepIMDB(&imdb, ""); *got != "tt9253284" {
		t.Errorf("keepIMDB wiped the id: %q", *got)
	}
}
