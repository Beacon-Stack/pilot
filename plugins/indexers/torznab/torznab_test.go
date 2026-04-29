package torznab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
