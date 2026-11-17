package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// Wanted > Missing lists only monitored episodes of monitored series. It
// used to include every monitored episode of unmonitored series too.
func TestWantedMissing_SkipsUnmonitoredSeries(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	db, err := sql.Open("sqlite", e.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp', 'Any', ?, ?)`, now, now)
	exec(`INSERT INTO libraries (id, name, root_path, default_quality_profile_id, created_at, updated_at) VALUES ('lib', 'TV', '/tv', 'qp', ?, ?)`, now, now)
	for i, s := range []struct {
		id        string
		monitored bool
	}{{"watched", true}, {"dropped", false}} {
		exec(`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, monitored, added_at, updated_at)
		      VALUES (?, ?, ?, ?, 2020, 'lib', 'qp', ?, ?, ?)`, s.id, i+1, s.id, s.id, s.monitored, now, now)
		exec(`INSERT INTO seasons (id, series_id, season_number) VALUES (?, ?, 1)`, s.id+"-s1", s.id)
		exec(`INSERT INTO episodes (id, series_id, season_id, season_number, episode_number, title, air_date, monitored, has_file)
		      VALUES (?, ?, ?, 1, 1, 'Pilot', '2020-01-01', TRUE, FALSE)`, s.id+"-e1", s.id, s.id+"-s1")
	}

	resp, body := e.do(t, "GET", "/api/v1/wanted/missing", map[string]string{"X-Api-Key": routerTestKey}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var got struct {
		Total    int64 `json:"total"`
		Episodes []struct {
			SeriesID string `json:"series_id"`
		} `json:"episodes"`
	}
	_ = json.Unmarshal([]byte(body), &got)
	if got.Total != 1 || len(got.Episodes) != 1 || got.Episodes[0].SeriesID != "watched" {
		t.Fatalf("missing = %s, want only the monitored series' episode", body)
	}
}
