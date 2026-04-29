package haul

import (
	"context"
	"strings"
	"testing"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ── Add: empty download URL is rejected at the plugin layer ──────────────────

// When an indexer's torznab response is missing the <enclosure> tag (some
// scrapers — notably Pulse's Pirate Bay definition at the time of writing —
// produce this), the release reaches the downloader plugin with no URL.
// Without the early check, Haul itself would return a confusing
// "either uri or file must be provided" 422 that points at the wrong layer.
// The plugin must reject upfront with a clear, indexer-attributable error.
func TestAdd_EmptyDownloadURLReturnsActionableError(t *testing.T) {
	c := New(Config{URL: "http://haul:8484", APIKey: "k"})

	_, err := c.Add(context.Background(), plugin.Release{
		Title:       "Star Wars Andor S01 1080p",
		Indexer:     "The Pirate Bay",
		DownloadURL: "",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no download URL") {
		t.Errorf("error should mention 'no download URL': %v", err)
	}
	if !strings.Contains(err.Error(), `"The Pirate Bay"`) {
		t.Errorf("error should attribute the indexer that's missing data: %v", err)
	}
	if !strings.Contains(err.Error(), `"Star Wars Andor S01 1080p"`) {
		t.Errorf("error should include the release title for triage: %v", err)
	}
	// The error should NOT bubble up Haul's confusing 422 text. We're
	// catching this BEFORE we hit Haul.
	if strings.Contains(err.Error(), "either uri or file") {
		t.Errorf("error leaked Haul's 422 text — should be caught at plugin layer: %v", err)
	}
}

func TestAdd_WhitespaceOnlyDownloadURLIsRejected(t *testing.T) {
	// Whitespace-only download URLs (single space, tab, "\n", etc.)
	// must hit the same early-rejection path as empty strings — same
	// rationale: the operator gets a clear error attributed to the
	// indexer/release instead of Haul's opaque 422 "either uri or file"
	// text. Tested for `" "`, `"\t"`, `"\n"`, and a mix.
	c := New(Config{URL: "http://haul:8484", APIKey: "k"})

	for _, uri := range []string{" ", "\t", "\n", "  \t \n "} {
		_, err := c.Add(context.Background(), plugin.Release{
			Title:       "Bogus",
			Indexer:     "ZeroIndexer",
			DownloadURL: uri,
		})
		if err == nil {
			t.Errorf("DownloadURL %q: expected rejection, got nil error", uri)
			continue
		}
		if !strings.Contains(err.Error(), "no download URL") {
			t.Errorf("DownloadURL %q: error should mention 'no download URL': %v", uri, err)
		}
		if strings.Contains(err.Error(), "either uri or file") {
			t.Errorf("DownloadURL %q: error leaked Haul's 422 text — should be caught at plugin layer: %v", uri, err)
		}
	}
}

// Smoke test: a magnet URI passes the empty-URL guard and proceeds. We
// don't care about the rest of the flow here (it'd require a Haul mock);
// we're just locking down the gate.
func TestAdd_MagnetURIPassesGate(t *testing.T) {
	c := New(Config{URL: "http://127.0.0.1:1/never-listens", APIKey: "k"})
	// magnet bypasses resolveDownloadURL; the next failure is the HTTP
	// POST to Haul. We expect that failure to NOT mention "no download URL".
	_, err := c.Add(context.Background(), plugin.Release{
		Title:       "Test",
		Indexer:     "Whatever",
		DownloadURL: "magnet:?xt=urn:btih:abc",
	})
	if err == nil {
		// Reaching Haul (which doesn't exist) is the expected failure —
		// some kind of network error.
		t.Fatal("expected network error reaching unreachable Haul, got nil")
	}
	if strings.Contains(err.Error(), "no download URL") {
		t.Errorf("magnet URI should pass the empty-URL gate: %v", err)
	}
}
