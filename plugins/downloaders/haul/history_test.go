package haul

// Tests for the history-lookup client wired in Phase 3.
//
// LookupHistory's HTTP call is exercised by an integration-style
// httptest.Server test below; the query-string builder is unit-
// tested separately because it's the part that's most likely to
// drift if a new HistoryFilter field gets added without a matching
// query-key serializer.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── buildHistoryQueryString ──────────────────────────────────────────────────

func TestBuildHistoryQueryString_AllFields(t *testing.T) {
	q := buildHistoryQueryString(HistoryFilter{
		Service:        "pilot",
		InfoHash:       "abc",
		EpisodeID:      "ep-1",
		SeriesID:       "ser-1",
		MovieID:        "mov-1",
		TMDBID:         95479,
		Season:         1,
		Episode:        48,
		IncludeRemoved: true,
		Limit:          50,
	})
	for _, want := range []string{
		"service=pilot",
		"info_hash=abc",
		"episode_id=ep-1",
		"series_id=ser-1",
		"movie_id=mov-1",
		"tmdb_id=95479",
		"season=1",
		"episode=48",
		"include_removed=true",
		"limit=50",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("expected query to contain %q, got: %s", want, q)
		}
	}
}

// Empty filter → empty query string. The URL forms cleanly when
// concatenated with the base path.
func TestBuildHistoryQueryString_EmptyFilter(t *testing.T) {
	q := buildHistoryQueryString(HistoryFilter{})
	if q != "" {
		t.Errorf("empty filter should yield empty query; got %q", q)
	}
}

// IncludeRemoved=false (default) must NOT add the param — letting it
// default server-side. Pinning this so a future "always send it"
// refactor doesn't accidentally send `include_removed=false` and
// confuse a future server that treats presence-of-key as "true".
func TestBuildHistoryQueryString_IncludeRemovedFalseOmitted(t *testing.T) {
	q := buildHistoryQueryString(HistoryFilter{Service: "pilot"})
	if strings.Contains(q, "include_removed") {
		t.Errorf("IncludeRemoved=false should NOT appear in query; got %s", q)
	}
}

// ── LookupHistory (integration-ish via httptest.Server) ─────────────────────

// Headline behaviour: a happy-path lookup returns the items array.
// Pin this so the response-shape contract with Haul's
// /api/v1/history endpoint is enforced from both sides.
func TestLookupHistory_DecodesItemsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/history" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []HistoryRecord{
				{InfoHash: "abc", Name: "Foo", Requester: "pilot", TMDBID: 95479, Season: 1, Episode: 48},
			},
		})
	}))
	defer srv.Close()

	c := New(Config{URL: srv.URL})
	out, err := c.LookupHistory(context.Background(), HistoryFilter{
		Service: "pilot",
		TMDBID:  95479,
	})
	if err != nil {
		t.Fatalf("LookupHistory: %v", err)
	}
	if len(out) != 1 || out[0].InfoHash != "abc" || out[0].Episode != 48 {
		t.Errorf("got %+v", out)
	}
}

// Empty items array — the JJK-not-yet-downloaded case. Must return
// a non-nil empty slice (not nil) so callers can `for _, x := range`
// without nil-checking.
func TestLookupHistory_EmptyResultIsNonNilSlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []HistoryRecord{}})
	}))
	defer srv.Close()

	c := New(Config{URL: srv.URL})
	out, err := c.LookupHistory(context.Background(), HistoryFilter{Service: "pilot"})
	if err != nil {
		t.Fatalf("LookupHistory: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(out) != 0 {
		t.Errorf("expected empty slice; got %d", len(out))
	}
}

// 404 from /by-hash/:hash → nil + nil error. This is the "Haul has
// never seen this magnet" path — UI treats it as "no badge to show".
// Returning an error here would surface as a noisy toast in the UI.
func TestLookupHistoryByHash_404IsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(Config{URL: srv.URL})
	rec, err := c.LookupHistoryByHash(context.Background(), "abc")
	if err != nil {
		t.Fatalf("404 must not be an error; got %v", err)
	}
	if rec != nil {
		t.Errorf("404 must return nil record; got %+v", rec)
	}
}

// 200 with a record body → the parsed record.
func TestLookupHistoryByHash_DecodesRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(HistoryRecord{InfoHash: "abc", Name: "JJK 48"})
	}))
	defer srv.Close()

	c := New(Config{URL: srv.URL})
	rec, err := c.LookupHistoryByHash(context.Background(), "abc")
	if err != nil {
		t.Fatalf("LookupHistoryByHash: %v", err)
	}
	if rec == nil || rec.InfoHash != "abc" {
		t.Errorf("got %+v", rec)
	}
}
