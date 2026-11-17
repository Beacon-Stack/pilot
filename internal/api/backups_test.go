package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupsAPI(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	auth := map[string]string{"X-Api-Key": routerTestKey, "Content-Type": "application/json"}

	resp, body := e.do(t, "POST", "/api/v1/system/backups", auth, "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
		Size int64  `json:"size"`
	}
	_ = json.Unmarshal([]byte(body), &created)
	if created.Kind != "manual" || created.Size == 0 || !strings.HasPrefix(created.Name, "pilot-manual-") {
		t.Fatalf("created = %s", body)
	}

	_, body = e.do(t, "GET", "/api/v1/system/backups", auth, "")
	var list struct {
		Backups []struct{ Name string } `json:"backups"`
		Folder  string                  `json:"folder"`
	}
	_ = json.Unmarshal([]byte(body), &list)
	if len(list.Backups) != 1 || list.Backups[0].Name != created.Name || list.Folder != filepath.Join(filepath.Dir(e.dbPath), "backups") {
		t.Fatalf("list = %s", body)
	}

	resp, body = e.do(t, "GET", "/api/v1/system/backups/"+created.Name+"/download", auth, "")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(body, "SQLite format 3") {
		t.Fatalf("download: %d, %.20q", resp.StatusCode, body)
	}
	if resp, _ = e.do(t, "GET", "/api/v1/system/backups/..%2Fpilot.db/download", auth, ""); resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("traversal download: status %d", resp.StatusCode)
	}

	resp, body = e.do(t, "POST", "/api/v1/system/backups/"+created.Name+"/restore", auth, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d %s", resp.StatusCode, body)
	}
	if _, err := os.Stat(e.dbPath + ".restore"); err != nil {
		t.Fatalf("restore not staged: %v", err)
	}

	resp, body = e.do(t, "PUT", "/api/v1/system/backup-settings", auth, `{"interval_days":3,"retention_days":0}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid settings: %d %s", resp.StatusCode, body)
	}
	if resp, body = e.do(t, "PUT", "/api/v1/system/backup-settings", auth, `{"interval_days":3,"retention_days":10}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("settings: %d %s", resp.StatusCode, body)
	}
	_, body = e.do(t, "GET", "/api/v1/system/backup-settings", auth, "")
	if !strings.Contains(body, `"interval_days":3`) || !strings.Contains(body, `"retention_days":10`) {
		t.Fatalf("settings = %s", body)
	}

	if resp, _ = e.do(t, "DELETE", "/api/v1/system/backups/"+created.Name, auth, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if resp, _ = e.do(t, "DELETE", "/api/v1/system/backups/"+created.Name, auth, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete: %d", resp.StatusCode)
	}
}
