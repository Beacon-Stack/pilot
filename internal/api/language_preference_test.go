package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestLanguagePreferenceAPI(t *testing.T) {
	e := newTestEnv(t, routerTestKey)
	auth := map[string]string{"X-Api-Key": routerTestKey, "Content-Type": "application/json"}

	var got struct {
		Languages []string `json:"languages"`
		Available []string `json:"available"`
	}
	resp, body := e.do(t, "GET", "/api/v1/language-preference", auth, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status %d: %s", resp.StatusCode, body)
	}
	_ = json.Unmarshal([]byte(body), &got)
	if got.Languages == nil || len(got.Languages) != 0 || len(got.Available) == 0 || got.Available[0] != "english" {
		t.Fatalf("default preference = %s, want any language and a list of choices", body)
	}

	resp, body = e.do(t, "PUT", "/api/v1/language-preference", auth, `{"languages":["klingon"]}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid PUT status %d, want 422: %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PUT", "/api/v1/language-preference", auth, `{"languages":["english","german"]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status %d: %s", resp.StatusCode, body)
	}
	_, body = e.do(t, "GET", "/api/v1/language-preference", auth, "")
	_ = json.Unmarshal([]byte(body), &got)
	if !reflect.DeepEqual(got.Languages, []string{"english", "german"}) {
		t.Fatalf("saved preference not returned: %s", body)
	}
}
