package dbutil

import (
	"encoding/json"
	"testing"
)

func TestMergeSettings(t *testing.T) {
	existing := json.RawMessage(`{"url":"http://idx","api_key":"real-secret"}`)
	cases := []struct {
		name, in string
		want     map[string]string
	}{
		{"omitted secret is kept", `{"url":"http://new"}`,
			map[string]string{"url": "http://new", "api_key": "real-secret"}},
		{"masked secret echoed back is kept", `{"url":"http://idx","api_key":"***"}`,
			map[string]string{"url": "http://idx", "api_key": "real-secret"}},
		{"new secret wins", `{"api_key":"rotated"}`,
			map[string]string{"url": "http://idx", "api_key": "rotated"}},
		{"explicit empty string wins", `{"api_key":""}`,
			map[string]string{"url": "http://idx", "api_key": ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got map[string]string
			if err := json.Unmarshal(MergeSettings(existing, json.RawMessage(c.in)), &got); err != nil {
				t.Fatal(err)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// A masked value for a key the stored settings don't have is passed
// through unchanged; there is nothing to restore it from.
func TestMergeSettings_MaskedWithoutExisting(t *testing.T) {
	got := string(MergeSettings(json.RawMessage(`{}`), json.RawMessage(`{"token":"***"}`)))
	if got != `{"token":"***"}` {
		t.Fatalf("got %s", got)
	}
}
