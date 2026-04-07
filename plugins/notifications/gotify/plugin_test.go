package gotify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

func TestNotify_Success(t *testing.T) {
	var gotBody gotifyPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if tok := r.URL.Query().Get("token"); tok != "app-token" {
			t.Errorf("token param = %q, want app-token", tok)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Token: "app-token"},
		client: srv.Client(),
	}

	event := plugin.NotificationEvent{
		Type:      "import_complete",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Message:   "Imported: Inception",
	}

	if err := n.Notify(context.Background(), event); err != nil {
		t.Fatalf("Notify() = %v", err)
	}

	if gotBody.Priority != 5 {
		t.Errorf("priority = %d, want 5", gotBody.Priority)
	}
	if gotBody.Message != "Imported: Inception" {
		t.Errorf("message = %q", gotBody.Message)
	}
}

func TestNotify_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Token: "bad"},
		client: srv.Client(),
	}

	err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test", Message: "hi"})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestTest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body gotifyPayload
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body.Priority != 1 {
			t.Errorf("test priority = %d, want 1", body.Priority)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":2}`))
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Token: "tok"},
		client: srv.Client(),
	}

	if err := n.Test(context.Background()); err != nil {
		t.Fatalf("Test() = %v", err)
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	n := New(Config{URL: "http://gotify.local/", Token: "tok"})
	if n.cfg.URL != "http://gotify.local" {
		t.Errorf("URL = %q, want no trailing slash", n.cfg.URL)
	}
}

func TestSanitizer(t *testing.T) {
	raw := json.RawMessage(`{"url":"http://gotify.local","token":"secret"}`)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["token"] = json.RawMessage(`"***"`)
	out, _ := json.Marshal(m)
	var result map[string]string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if result["token"] != "***" {
		t.Errorf("token = %q, want ***", result["token"])
	}
	if result["url"] != "http://gotify.local" {
		t.Errorf("url was modified: %q", result["url"])
	}
}
