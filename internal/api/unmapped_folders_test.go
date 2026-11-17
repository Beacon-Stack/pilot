package api

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestUnmappedFoldersAPI(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	auth := map[string]string{"X-Api-Key": routerTestKey, "Content-Type": "application/json"}
	db, err := sql.Open("sqlite", e.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, q := range []string{
		`INSERT INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp', 'Any', '` + now + `', '` + now + `')`,
		`INSERT INTO libraries (id, name, root_path, default_quality_profile_id, created_at, updated_at) VALUES ('lib', 'TV', '/tv', 'qp', '` + now + `', '` + now + `')`,
		`INSERT INTO unmapped_folders (id, library_id, path, name, guess_tmdb_id, guess_title, guess_year, first_seen_at) VALUES ('u1', 'lib', '/tv/Breaking Bad', 'Breaking Bad', 1396, 'Breaking Bad', 2008, '` + now + `')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	resp, body := e.do(t, "GET", "/api/v1/library/unmapped-folders", auth, "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"guess_tmdb_id":1396`) {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	if resp, body = e.do(t, "POST", "/api/v1/library/unmapped-folders/u1/ignore", auth, `{"ignored":true}`); resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("ignore: %d %s", resp.StatusCode, body)
	}
	if _, body = e.do(t, "GET", "/api/v1/library/unmapped-folders", auth, ""); strings.Contains(body, "u1") {
		t.Fatalf("ignored folder listed: %s", body)
	}
	if _, body = e.do(t, "GET", "/api/v1/library/unmapped-folders?include_ignored=true", auth, ""); !strings.Contains(body, `"ignored":true`) {
		t.Fatalf("include_ignored: %s", body)
	}
	if resp, _ = e.do(t, "POST", "/api/v1/library/unmapped-folders/nope/ignore", auth, `{"ignored":true}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown id: %d", resp.StatusCode)
	}
}
