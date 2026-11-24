package pulse

import (
	"encoding/json"
	"testing"

	"github.com/beacon-stack/pulse/pkg/sdk"
)

// The secrets Pulse hands over end up where the downloader plugins read
// them, so the app can reach a client that requires a login.
func TestBuildDownloadClientSettings_CarriesSecrets(t *testing.T) {
	settings := func(c sdk.DownloadClient) map[string]any {
		var m map[string]any
		if err := json.Unmarshal(buildDownloadClientSettings(c), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	haul := settings(sdk.DownloadClient{Kind: "haul", Host: "vpn", Port: 8484, APIKey: "hauls-own-key"})
	if haul["api_key"] != "hauls-own-key" || haul["url"] != "http://vpn:8484" {
		t.Errorf("haul settings: %v", haul)
	}
	if _, ok := haul["password"]; ok {
		t.Errorf("no password was given, none should be stored: %v", haul)
	}

	qbit := settings(sdk.DownloadClient{Kind: "qbittorrent", Host: "qbit", Port: 8080, Username: "admin", Password: "hunter2hunter2"})
	if qbit["password"] != "hunter2hunter2" || qbit["username"] != "admin" {
		t.Errorf("qbittorrent settings: %v", qbit)
	}

	// Nothing from Pulse: the settings are what they were before.
	plain := settings(sdk.DownloadClient{Kind: "haul", Host: "vpn", Port: 8484})
	if _, ok := plain["api_key"]; ok {
		t.Errorf("no key was given, none should be stored: %v", plain)
	}
}
