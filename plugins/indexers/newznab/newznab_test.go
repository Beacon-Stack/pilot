package newznab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ── Newznab <error> document handling ───────────────────────────────────────

// Mirror of the torznab plugin's regression guard (see torznab_test.go for
// the 2026-08-16 incident background): an <error> document served with
// HTTP 200 must surface its code and description, never an XML
// type-mismatch about <rss>.
const newznabErrorDoc = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<error code="100" description="Incorrect user credentials"></error>`

func errorDocServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(newznabErrorDoc))
	}))
}

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
		t.Errorf("error %q should contain the newznab error code", msg)
	}
	if !strings.Contains(msg, "Incorrect user credentials") {
		t.Errorf("error %q should contain the indexer's description", msg)
	}
	if strings.Contains(msg, "expected element type") {
		t.Errorf("error %q leaks the raw XML mismatch — the error doc was not parsed", msg)
	}
}

// Test used to check only the HTTP status, so an HTTP-200 <error> document
// (e.g. a bad API key) reported success. It must fail with the description.
func TestTest_ErrorDocumentFailsTest(t *testing.T) {
	srv := errorDocServer()
	defer srv.Close()

	idx := New(Config{URL: srv.URL, APIKey: "k"})

	err := idx.Test(context.Background())
	if err == nil {
		t.Fatal("Test() on an <error> document = nil, want the indexer's error")
	}
	if !strings.Contains(err.Error(), "Incorrect user credentials") {
		t.Errorf("error %q should contain the indexer's description", err)
	}
}

// An HTTP-200 empty body still passes Test, matching the torznab plugin.
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
