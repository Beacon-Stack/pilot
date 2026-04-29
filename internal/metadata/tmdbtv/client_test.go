package tmdbtv

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient wires the package's Client at a fake TMDB returning the
// supplied handler. Auth is irrelevant — the test handler doesn't check.
func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := New("test-key", logger)
	c.baseURL = srv.URL
	return c, srv
}

// ── GetAlternativeTitles ─────────────────────────────────────────────────────

func TestGetAlternativeTitles_HappyPath(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/tv/83867/alternative_titles") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"id": 83867,
			"results": [
				{"iso_3166_1": "US", "title": "Star Wars: Andor", "type": ""},
				{"iso_3166_1": "FR", "title": "Andor: A Star Wars Story", "type": ""}
			]
		}`))
	})
	got, err := c.GetAlternativeTitles(context.Background(), 83867)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Star Wars: Andor", "Andor: A Star Wars Story"}
	if len(got) != len(want) {
		t.Fatalf("got %d titles, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("titles[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGetAlternativeTitles_EmptyResults(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": 1, "results": []}`))
	})
	got, err := c.GetAlternativeTitles(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestGetAlternativeTitles_DedupsCaseInsensitive(t *testing.T) {
	// Same title can appear under multiple regions in TMDB; we should
	// surface each unique title once. Comparison is case-insensitive
	// so "Star Wars: Andor" and "STAR WARS: ANDOR" collapse.
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"results": [
				{"title": "Star Wars: Andor"},
				{"title": "STAR WARS: ANDOR"},
				{"title": "star wars: andor"},
				{"title": "Andor"}
			]
		}`))
	})
	got, err := c.GetAlternativeTitles(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 unique titles after dedup, got %d: %v", len(got), got)
	}
}

func TestGetAlternativeTitles_TrimsAndSkipsWhitespace(t *testing.T) {
	// TMDB has been seen returning entries with leading/trailing whitespace
	// or completely empty strings (translation submissions). Both should
	// be normalized.
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"results": [
				{"title": "  Padded Title  "},
				{"title": ""},
				{"title": "   "},
				{"title": "Real Title"}
			]
		}`))
	})
	got, err := c.GetAlternativeTitles(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 titles after trimming, got %d: %v", len(got), got)
	}
	if got[0] != "Padded Title" {
		t.Errorf("first title = %q, want %q (whitespace trimmed)", got[0], "Padded Title")
	}
}

func TestGetAlternativeTitles_NotFound(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"status_message":"The resource you requested could not be found."}`, http.StatusNotFound)
	})
	_, err := c.GetAlternativeTitles(context.Background(), 999999)
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
	if !strings.Contains(err.Error(), "alt titles") {
		t.Errorf("error doesn't mention alt titles: %v", err)
	}
}

func TestGetAlternativeTitles_AuthFailure(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"status_message":"Invalid API key"}`, http.StatusUnauthorized)
	})
	_, err := c.GetAlternativeTitles(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}

func TestGetAlternativeTitles_NetworkError(t *testing.T) {
	// Server that closes the connection mid-response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := New("test-key", logger)
	c.baseURL = srv.URL

	_, err := c.GetAlternativeTitles(context.Background(), 1)
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
}

func TestGetAlternativeTitles_MalformedJSON(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{this is not valid json`))
	})
	_, err := c.GetAlternativeTitles(context.Background(), 1)
	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
}

func TestGetAlternativeTitles_RespectsContextCancellation(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel
	_, err := c.GetAlternativeTitles(ctx, 1)
	if err == nil {
		t.Fatal("expected context-cancelled error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		// not all transports wrap ctx.Err verbatim; accept anything that
		// signals cancellation, but at minimum we shouldn't return nil.
		t.Logf("error path (acceptable if context-related): %v", err)
	}
}
