package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mmBody struct {
	RenameEpisodes        bool    `json:"rename_episodes"`
	StandardEpisodeFormat string  `json:"standard_episode_format"`
	RecycleBinPath        *string `json:"recycle_bin_path"`
	RecycleBinCleanupDays *int    `json:"recycle_bin_cleanup_days"`
}

func TestRecycleBinSettingsAPI(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	auth := map[string]string{"X-Api-Key": routerTestKey, "Content-Type": "application/json"}

	resp, body := e.do(t, "GET", "/api/v1/media-management", auth, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET: %d %s", resp.StatusCode, body)
	}
	var full map[string]any
	_ = json.Unmarshal([]byte(body), &full)
	if full["recycle_bin_path"] != "" || full["recycle_bin_cleanup_days"] != float64(7) {
		t.Fatalf("defaults = %s", body)
	}

	put := func(change map[string]any) (result, string) {
		b := map[string]any{}
		for k, v := range full {
			b[k] = v
		}
		for k, v := range change {
			if v == nil {
				delete(b, k)
			} else {
				b[k] = v
			}
		}
		raw, _ := json.Marshal(b)
		return e.do(t, "PUT", "/api/v1/media-management", auth, string(raw))
	}

	bin := filepath.Join(t.TempDir(), "recycle")
	if resp, body = put(map[string]any{"recycle_bin_path": bin, "recycle_bin_cleanup_days": 14}); resp.StatusCode != http.StatusOK {
		t.Fatalf("set bin: %d %s", resp.StatusCode, body)
	}
	// An older client that omits the fields must not turn the bin off.
	if resp, body = put(map[string]any{"recycle_bin_path": nil, "recycle_bin_cleanup_days": nil, "rename_episodes": false}); resp.StatusCode != http.StatusOK {
		t.Fatalf("put without bin fields: %d %s", resp.StatusCode, body)
	}
	_, body = e.do(t, "GET", "/api/v1/media-management", auth, "")
	var got mmBody
	_ = json.Unmarshal([]byte(body), &got)
	if got.RecycleBinPath == nil || *got.RecycleBinPath != bin || *got.RecycleBinCleanupDays != 14 || got.RenameEpisodes {
		t.Fatalf("after partial update = %s", body)
	}

	if resp, body = put(map[string]any{"recycle_bin_path": "relative/bin"}); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("relative bin: %d %s", resp.StatusCode, body)
	}

	// Inside a library: a library scan would re-import recycled files.
	root := filepath.Join(t.TempDir(), "tv")
	db, err := sql.Open("sqlite", e.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp', 'Any', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO libraries (id, name, root_path, default_quality_profile_id, created_at, updated_at) VALUES ('lib', 'TV', ?, 'qp', ?, ?)`, root, now, now); err != nil {
		t.Fatal(err)
	}
	resp, body = put(map[string]any{"recycle_bin_path": filepath.Join(root, ".recycle")})
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "TV") {
		t.Fatalf("bin inside library: %d %s", resp.StatusCode, body)
	}

	// Empty the bin.
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(bin, "old.mkv"), []byte("x"), 0o644)
	resp, body = e.do(t, "POST", "/api/v1/recycle-bin/empty", auth, "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"deleted":1`) {
		t.Fatalf("empty: %d %s", resp.StatusCode, body)
	}
}
