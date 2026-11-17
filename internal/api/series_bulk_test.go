package api

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSeriesBulkAPI(t *testing.T) {
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
		`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, added_at, updated_at) VALUES ('a', 1, 'A', 'a', 2020, 'lib', 'qp', '` + now + `', '` + now + `')`,
		`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, added_at, updated_at) VALUES ('b', 2, 'B', 'b', 2020, 'lib', 'qp', '` + now + `', '` + now + `')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	// The static /bulk path must not be taken for a series id.
	resp, body := e.do(t, "PUT", "/api/v1/series/bulk", auth, `{"ids":["a","b"],"monitored":false,"series_type":"anime"}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"count":2`) {
		t.Fatalf("edit: %d %s", resp.StatusCode, body)
	}
	var monitored bool
	var typ string
	_ = db.QueryRow(`SELECT monitored, series_type FROM series WHERE id = 'b'`).Scan(&monitored, &typ)
	if monitored || typ != "anime" {
		t.Fatalf("b = monitored %v, type %q", monitored, typ)
	}

	if resp, body = e.do(t, "PUT", "/api/v1/series/bulk", auth, `{"ids":["a","ghost"],"monitored":true}`); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown id: %d %s", resp.StatusCode, body)
	}
	_ = db.QueryRow(`SELECT monitored FROM series WHERE id = 'a'`).Scan(&monitored)
	if monitored {
		t.Fatal("a changed although the request was rejected")
	}

	resp, body = e.do(t, "POST", "/api/v1/series/bulk/delete", auth, `{"ids":["a"]}`)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"count":1`) {
		t.Fatalf("delete: %d %s", resp.StatusCode, body)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM series`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d series left, want 1", n)
	}
}
