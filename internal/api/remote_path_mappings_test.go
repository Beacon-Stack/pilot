package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRemotePathMappingsAPI(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	auth := map[string]string{"X-Api-Key": routerTestKey, "Content-Type": "application/json"}

	resp, body := e.do(t, "POST", "/api/v1/remote-path-mappings", auth, `{"remote_path":"downloads","local_path":"/mnt/dl"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("relative remote path: status %d, want 422: %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/v1/remote-path-mappings", auth, `{"download_client_id":"ghost","remote_path":"/downloads","local_path":"/mnt/dl"}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown client: status %d, want 422: %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "POST", "/api/v1/remote-path-mappings", auth, `{"remote_path":"/downloads","local_path":"/mnt/dl"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d: %s", resp.StatusCode, body)
	}
	var created struct {
		ID         string `json:"id"`
		RemotePath string `json:"remote_path"`
	}
	_ = json.Unmarshal([]byte(body), &created)

	resp, body = e.do(t, "PUT", "/api/v1/remote-path-mappings/"+created.ID, auth, `{"remote_path":"/data","local_path":"/mnt/data"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: status %d: %s", resp.StatusCode, body)
	}
	_, body = e.do(t, "GET", "/api/v1/remote-path-mappings", auth, "")
	var list []struct {
		RemotePath string `json:"remote_path"`
		LocalPath  string `json:"local_path"`
	}
	_ = json.Unmarshal([]byte(body), &list)
	if len(list) != 1 || list[0].RemotePath != "/data" || list[0].LocalPath != "/mnt/data" {
		t.Fatalf("list = %s", body)
	}

	if resp, body = e.do(t, "DELETE", "/api/v1/remote-path-mappings/"+created.ID, auth, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d: %s", resp.StatusCode, body)
	}
	if resp, _ = e.do(t, "DELETE", "/api/v1/remote-path-mappings/"+created.ID, auth, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete: status %d, want 404", resp.StatusCode)
	}
}
