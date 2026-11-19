package wanted

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/quality"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

type fakeProfiles map[string]quality.Profile

func (f fakeProfiles) Get(_ context.Context, id string) (quality.Profile, error) {
	p, ok := f[id]
	if !ok {
		return quality.Profile{}, errors.New("not found")
	}
	return p, nil
}

func qjson(t *testing.T, q plugin.Quality) string {
	t.Helper()
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var (
	web720  = plugin.Quality{Resolution: plugin.Resolution720p, Source: plugin.SourceWEBDL}
	web1080 = plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL}
	bd1080  = plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceBluRay}
)

func TestCutoffUnmet(t *testing.T) {
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := d.SQL.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	const now = "2026-11-17T00:00:00Z"
	exec(`INSERT INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp', 'HD', ?, ?)`, now, now)
	exec(`INSERT INTO libraries (id, name, root_path, default_quality_profile_id, created_at, updated_at) VALUES ('lib', 'TV', '/tv', 'qp', ?, ?)`, now, now)
	for _, s := range []struct {
		id, title string
		monitored bool
	}{{"s1", "Alpha", true}, {"s2", "Beta", false}} {
		exec(`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, monitored, added_at, updated_at)
		      VALUES (?, ?, ?, ?, 2020, 'lib', 'qp', ?, ?, ?)`, s.id, len(s.id)+int(s.title[0]), s.title, s.title, s.monitored, now, now)
		exec(`INSERT INTO seasons (id, series_id, season_number) VALUES (?, ?, 1)`, s.id+"-season", s.id)
	}
	episodes := []struct {
		id, series string
		num        int
		monitored  bool
		file       *plugin.Quality
	}{
		{"e-below", "s1", 2, true, &web720},             // listed
		{"e-below-2", "s1", 1, true, &web720},           // listed, sorts first
		{"e-at", "s1", 3, true, &bd1080},                // at cutoff
		{"e-unknown", "s1", 4, true, &plugin.Quality{}}, // unknown quality
		{"e-unmon", "s1", 5, false, &web720},            // unmonitored episode
		{"e-missing", "s1", 6, true, nil},               // no file
		{"e-series-off", "s2", 1, true, &web720},        // unmonitored series
	}
	for _, e := range episodes {
		exec(`INSERT INTO episodes (id, series_id, season_id, season_number, episode_number, title, monitored, has_file)
		      VALUES (?, ?, ?, 1, ?, 'Ep', ?, ?)`, e.id, e.series, e.series+"-season", e.num, e.monitored, e.file != nil)
		if e.file != nil {
			exec(`INSERT INTO episode_files (id, episode_id, series_id, path, size_bytes, quality_json, imported_at, indexed_at)
			      VALUES (?, ?, ?, ?, 1, ?, ?, ?)`, "f-"+e.id, e.id, e.series, "/tv/"+e.id+".mkv", qjson(t, *e.file), now, now)
		}
	}

	got, err := CutoffUnmet(context.Background(), db.New(d.SQL), fakeProfiles{"qp": {Cutoff: bd1080}})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, ce := range got {
		ids = append(ids, ce.Episode.ID)
	}
	if len(ids) != 2 || ids[0] != "e-below-2" || ids[1] != "e-below" {
		t.Fatalf("cutoff unmet = %v, want [e-below-2 e-below]", ids)
	}
	if got[0].Current.Resolution != plugin.Resolution720p || got[0].Cutoff.Source != plugin.SourceBluRay {
		t.Fatalf("qualities not reported: %+v", got[0])
	}

	// A profile with no cutoff lists nothing.
	none, err := CutoffUnmet(context.Background(), db.New(d.SQL), fakeProfiles{"qp": {}})
	if err != nil || len(none) != 0 {
		t.Fatalf("no-cutoff profile: %v %v", none, err)
	}
}

func TestBestFileQuality(t *testing.T) {
	files := []db.EpisodeFile{
		{EpisodeID: "e1", QualityJson: qjson(t, web720)},
		{EpisodeID: "e1", QualityJson: qjson(t, web1080)},
		{EpisodeID: "e2", QualityJson: qjson(t, bd1080)},
	}
	if got := BestFileQuality(files, "e1"); got == nil || got.Resolution != plugin.Resolution1080p {
		t.Fatalf("best for e1 = %+v, want 1080p", got)
	}
	if got := BestFileQuality(files, "e3"); got != nil {
		t.Fatalf("episode without files = %+v, want nil", got)
	}
	proper := web1080
	proper.Revision = 2
	files = append(files, db.EpisodeFile{EpisodeID: "e1", QualityJson: qjson(t, proper)})
	if got := BestFileQuality(files, "e1"); got.EffectiveRevision() != 2 {
		t.Fatalf("same quality, higher revision should win; got revision %d", got.EffectiveRevision())
	}
}
