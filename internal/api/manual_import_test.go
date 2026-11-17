package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManualImportAPI(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	auth := map[string]string{"X-Api-Key": routerTestKey, "Content-Type": "application/json"}
	root := filepath.Join(t.TempDir(), "tv")
	incoming := filepath.Join(t.TempDir(), "incoming")
	_ = os.MkdirAll(root, 0o755)
	_ = os.MkdirAll(incoming, 0o755)
	src := filepath.Join(incoming, "Show.S01E01.1080p.WEB-DL.mkv")
	_ = os.WriteFile(src, []byte("video"), 0o644)

	db, err := sql.Open("sqlite", e.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, q := range []string{
		`INSERT INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp', 'Any', '` + now + `', '` + now + `')`,
		`INSERT INTO libraries (id, name, root_path, default_quality_profile_id, created_at, updated_at) VALUES ('lib', 'TV', '` + root + `', 'qp', '` + now + `', '` + now + `')`,
		`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, added_at, updated_at) VALUES ('s', 1, 'Show', 'show', 2022, 'lib', 'qp', '` + now + `', '` + now + `')`,
		`INSERT INTO seasons (id, series_id, season_number) VALUES ('s1', 's', 1)`,
		`INSERT INTO episodes (id, series_id, season_id, season_number, episode_number, title) VALUES ('e1', 's', 's1', 1, 1, 'Pilot')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	if resp, body := e.do(t, "POST", "/api/v1/manual-import/scan", auth, `{"path":"relative"}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("relative path: %d %s", resp.StatusCode, body)
	}

	resp, body := e.do(t, "POST", "/api/v1/manual-import/scan", auth, fmt.Sprintf(`{"path":%q}`, incoming))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scan: %d %s", resp.StatusCode, body)
	}
	var scan struct {
		Candidates []struct {
			Path     string                `json:"path"`
			SeriesID string                `json:"series_id"`
			Episodes []struct{ ID string } `json:"episodes"`
			Quality  json.RawMessage       `json:"quality"`
		} `json:"candidates"`
	}
	_ = json.Unmarshal([]byte(body), &scan)
	if len(scan.Candidates) != 1 || scan.Candidates[0].SeriesID != "s" || len(scan.Candidates[0].Episodes) != 1 {
		t.Fatalf("scan = %s", body)
	}

	c := scan.Candidates[0]
	req := fmt.Sprintf(`{"mode":"copy","items":[{"path":%q,"series_id":"s","episode_ids":["e1"],"quality":%s}]}`, c.Path, c.Quality)
	resp, body = e.do(t, "POST", "/api/v1/manual-import", auth, req)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("import: %d %s", resp.StatusCode, body)
	}
	var path string
	if err := db.QueryRow(`SELECT path FROM episode_files WHERE episode_id = 'e1'`).Scan(&path); err != nil || !strings.HasPrefix(path, root) {
		t.Fatalf("library file = %q, %v", path, err)
	}
}
