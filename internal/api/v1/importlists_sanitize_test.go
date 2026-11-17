package v1

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/beacon-stack/pilot/internal/core/importlist"

	_ "github.com/beacon-stack/pilot/plugins/importlists/plex_watchlist_tv"
	_ "github.com/beacon-stack/pilot/plugins/importlists/trakt_list_tv"
)

// Regression: import list responses used to return settings verbatim,
// exposing the Plex account token to anyone who could read the API.
func TestImportListToBody_MasksPlexToken(t *testing.T) {
	const token = "plex-account-token-abcdef123456"
	body := importListToBody(importlist.Config{
		Kind:     "plex_watchlist_tv",
		Settings: json.RawMessage(`{"access_token":"` + token + `"}`),
	})
	if strings.Contains(string(body.Settings), token) {
		t.Fatalf("Plex token leaked in API response: %s", body.Settings)
	}
	if !strings.Contains(string(body.Settings), `"access_token":"***"`) {
		t.Fatalf("expected masked access_token, got %s", body.Settings)
	}
}

// Non-secret settings must survive so the edit form can pre-fill them.
func TestImportListToBody_KeepsNonSecretSettings(t *testing.T) {
	body := importListToBody(importlist.Config{
		Kind:     "trakt_list_tv",
		Settings: json.RawMessage(`{"username":"someone","list_type":"watchlist"}`),
	})
	if !strings.Contains(string(body.Settings), `"username":"someone"`) {
		t.Fatalf("non-secret settings were dropped: %s", body.Settings)
	}
}

// Unknown kinds fail closed: no sanitizer registered means nothing is shown.
func TestImportListToBody_UnknownKindFailsClosed(t *testing.T) {
	body := importListToBody(importlist.Config{
		Kind:     "not_a_real_kind",
		Settings: json.RawMessage(`{"secret":"x"}`),
	})
	if string(body.Settings) != "{}" {
		t.Fatalf("unknown kind returned settings: %s", body.Settings)
	}
}
