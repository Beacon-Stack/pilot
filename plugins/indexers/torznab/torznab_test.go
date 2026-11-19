package torznab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ── Search query construction ───────────────────────────────────────────────

// Regression guard: q.Query MUST be sent verbatim to the indexer when
// issuing a tvsearch. We learned the hard way that many torznab
// implementations (TorrentGalaxy is the canonical example) silently
// ignore the structured `season=` parameter and only text-match `q`.
// If we strip "S01" / "Season N" from q hoping the structured param
// will filter, those indexers fall back to fuzzy substring matching
// — searching for "Andor" returns "Anderson", "Andrea", etc. and
// drops the real Andor releases entirely.
//
// Keep this test green: it locks down that "Andor S01" stays "Andor S01"
// in the outgoing q parameter even when season=1 is also set.
func TestSearch_TVSearchSendsQVerbatim(t *testing.T) {
	var capturedURLs []*url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURLs = append(capturedURLs, &u)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
	}))
	defer srv.Close()

	idx := New(Config{URL: srv.URL, APIKey: "k"})

	_, _ = idx.Search(context.Background(), plugin.SearchQuery{
		Query:  "Andor S01",
		Season: 1,
	})

	if len(capturedURLs) == 0 {
		t.Fatal("server never received a request")
	}
	first := capturedURLs[0]
	if got := first.Query().Get("t"); got != "tvsearch" {
		t.Fatalf("first request t param = %q, want tvsearch", got)
	}
	if q := first.Query().Get("q"); q != "Andor S01" {
		t.Errorf("q sent to indexer = %q, want %q (must NOT be stripped — see comment)", q, "Andor S01")
	}
	if got := first.Query().Get("season"); got != "1" {
		t.Errorf("season param = %q, want %q (structured filter belt-and-braces)", got, "1")
	}
}

// Counterpart: when season=0 (whole-series search), only `t=search` is
// issued — no tvsearch attempt — and q passes through unchanged.
func TestSearch_TextSearchSingleRequest(t *testing.T) {
	var capturedURLs []*url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURLs = append(capturedURLs, &u)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
	}))
	defer srv.Close()

	idx := New(Config{URL: srv.URL, APIKey: "k"})

	_, _ = idx.Search(context.Background(), plugin.SearchQuery{
		Query:  "Andor",
		Season: 0,
	})

	if len(capturedURLs) != 1 {
		t.Fatalf("expected exactly 1 request (text-search only), got %d", len(capturedURLs))
	}
	got := capturedURLs[0]
	if q := got.Query().Get("q"); q != "Andor" {
		t.Errorf("q = %q, want Andor", q)
	}
	if tp := got.Query().Get("t"); tp != "search" {
		t.Errorf("t param = %q, want search", tp)
	}
}

// When tvsearch returns zero items, Search MUST fall through to text
// search so indexers that don't support tvsearch (or can't find
// anything via it) still get a chance to respond.
func TestSearch_FallsBackToTextSearchOnZeroTVResults(t *testing.T) {
	var capturedURLs []*url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		capturedURLs = append(capturedURLs, &u)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
	}))
	defer srv.Close()

	idx := New(Config{URL: srv.URL, APIKey: "k"})

	_, _ = idx.Search(context.Background(), plugin.SearchQuery{
		Query:  "Andor S01",
		Season: 1,
	})

	if len(capturedURLs) != 2 {
		t.Fatalf("expected 2 requests (tvsearch + text fallback), got %d", len(capturedURLs))
	}
	if got := capturedURLs[0].Query().Get("t"); got != "tvsearch" {
		t.Errorf("first request t = %q, want tvsearch", got)
	}
	if got := capturedURLs[1].Query().Get("t"); got != "search" {
		t.Errorf("second request t = %q, want search (fallback)", got)
	}
	// Both must carry the full query verbatim.
	if got := capturedURLs[1].Query().Get("q"); got != "Andor S01" {
		t.Errorf("text-fallback q = %q, want %q", got, "Andor S01")
	}
}

// ── Torznab <error> document handling ───────────────────────────────────────

// torznabErrorDoc is the exact document Pulse returned during the
// 2026-08-16 incident: the upstream Prowlarr definition for
// TorrentGalaxyClone was removed (TorrentGalaxy shut down), and Pulse
// answered every search with this HTTP-200 error document. Pilot decoded
// it blindly into the RSS struct and reported the useless
// `expected element type <rss> but have <error>` — hiding the actual
// reason for weeks of "indexes are broken" reports.
const torznabErrorDoc = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<error code="100" description="No catalog definition found for indexer &#34;TorrentGalaxyClone&#34; — it may have been removed from Prowlarr"></error>`

func errorDocServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(torznabErrorDoc))
	}))
}

// Regression guard: a torznab <error> document (the protocol's standard
// failure signaling, served with HTTP 200) MUST surface its code and
// description, never an XML type-mismatch about <rss>.
func TestSearch_ErrorDocumentSurfacesDescription(t *testing.T) {
	srv := errorDocServer()
	defer srv.Close()

	idx := New(Config{URL: srv.URL, APIKey: "k"})

	_, err := idx.Search(context.Background(), plugin.SearchQuery{Query: "Ted Lasso S04E05"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "indexer reported error 100") {
		t.Errorf("error %q should contain the torznab error code", msg)
	}
	if !strings.Contains(msg, "No catalog definition found") {
		t.Errorf("error %q should contain the indexer's description", msg)
	}
	if strings.Contains(msg, "expected element type") {
		t.Errorf("error %q leaks the raw XML mismatch — the error doc was not parsed", msg)
	}
}

// The connectivity Test must also surface the description — this is the
// message shown verbatim in the UI's per-indexer Test button, so it is the
// most direct place a user can diagnose a broken indexer.
func TestTest_ErrorDocumentSurfacesDescription(t *testing.T) {
	srv := errorDocServer()
	defer srv.Close()

	idx := New(Config{URL: srv.URL, APIKey: "k"})

	err := idx.Test(context.Background())
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "No catalog definition found") {
		t.Errorf("error %q should contain the indexer's description", msg)
	}
	if strings.Contains(msg, "wrong URL?") {
		t.Errorf("error %q gives the misleading wrong-URL hint for a parsed error doc", msg)
	}
}

func TestCapabilities_ErrorDocumentSurfacesDescription(t *testing.T) {
	srv := errorDocServer()
	defer srv.Close()

	idx := New(Config{URL: srv.URL, APIKey: "k"})

	_, err := idx.Capabilities(context.Background())
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "No catalog definition found") {
		t.Errorf("error %q should contain the indexer's description", err)
	}
}

// Preserved behavior: an HTTP-200 empty body still passes Test — some
// Prowlarr-backed indexers return no caps document while serving search
// results fine.
func TestTest_EmptyBodyStillPasses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	idx := New(Config{URL: srv.URL, APIKey: "k"})

	if err := idx.Test(context.Background()); err != nil {
		t.Fatalf("Test() on empty 200 body = %v, want nil", err)
	}
}
