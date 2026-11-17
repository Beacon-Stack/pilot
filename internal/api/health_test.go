package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestHealthAPI(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	auth := map[string]string{"X-Api-Key": routerTestKey}

	for _, req := range []struct{ method, path string }{
		{"GET", "/api/v1/system/health"},
		{"POST", "/api/v1/system/health/check"},
	} {
		resp, body := e.do(t, req.method, req.path, auth, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %s: status %d: %s", req.method, req.path, resp.StatusCode, body)
		}
		var got struct {
			Issues []struct {
				Source, Level, Message string
			} `json:"issues"`
			ConnectivityCheckedAt string `json:"connectivity_checked_at"`
		}
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatal(err)
		}
		// The test env has no library.
		if len(got.Issues) == 0 || got.Issues[0].Source != "library" || !strings.Contains(got.Issues[0].Message, "No library") {
			t.Fatalf("%s %s: issues = %s", req.method, req.path, body)
		}
		if got.ConnectivityCheckedAt == "" {
			t.Fatalf("%s %s: connectivity never checked: %s", req.method, req.path, body)
		}
	}
}
