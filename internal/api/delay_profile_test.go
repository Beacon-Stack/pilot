package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestDelayProfileAPI(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	auth := map[string]string{"X-Api-Key": routerTestKey, "Content-Type": "application/json"}

	resp, body := e.do(t, "GET", "/api/v1/delay-profile", auth, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status %d: %s", resp.StatusCode, body)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(body), &got)
	if got["preferred_protocol"] != "usenet" || got["usenet_enabled"] != true || got["torrent_delay_minutes"] != float64(0) {
		t.Fatalf("default profile = %s", body)
	}

	resp, body = e.do(t, "PUT", "/api/v1/delay-profile", auth,
		`{"usenet_enabled":false,"torrent_enabled":false,"preferred_protocol":"usenet","usenet_delay_minutes":0,"torrent_delay_minutes":0,"bypass_if_highest_quality":true}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid PUT status %d, want 422: %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PUT", "/api/v1/delay-profile", auth,
		`{"usenet_enabled":true,"torrent_enabled":true,"preferred_protocol":"torrent","usenet_delay_minutes":15,"torrent_delay_minutes":120,"bypass_if_highest_quality":false}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status %d: %s", resp.StatusCode, body)
	}
	_, body = e.do(t, "GET", "/api/v1/delay-profile", auth, "")
	_ = json.Unmarshal([]byte(body), &got)
	if got["preferred_protocol"] != "torrent" || got["torrent_delay_minutes"] != float64(120) || got["bypass_if_highest_quality"] != false {
		t.Fatalf("saved profile not returned: %s", body)
	}

	resp, body = e.do(t, "GET", "/api/v1/queue/pending", auth, "")
	if resp.StatusCode != http.StatusOK || body == "null" {
		t.Fatalf("pending list: %d %s", resp.StatusCode, body)
	}
}
