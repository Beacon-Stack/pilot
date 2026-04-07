package ntfy

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/pilot" {
			t.Errorf("path = %q, want /pilot", r.URL.Path)
		}
		if title := r.Header.Get("Title"); title == "" {
			t.Error("missing Title header")
		}
		if pri := r.Header.Get("Priority"); pri != "3" {
			t.Errorf("priority = %q, want 3", pri)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer my-token" {
			t.Errorf("auth = %q, want Bearer my-token", auth)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "Grabbing: Inception" {
			t.Errorf("body = %q", string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Topic: "pilot", Token: "my-token", Priority: 3},
		client: srv.Client(),
	}

	event := plugin.NotificationEvent{
		Type:      "grab_started",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Message:   "Grabbing: Inception",
	}

	if err := n.Notify(context.Background(), event); err != nil {
		t.Fatalf("Notify() = %v", err)
	}
}

func TestNotify_NoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("unexpected auth header: %q", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Topic: "test", Priority: 3},
		client: srv.Client(),
	}

	if err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test", Message: "hi"}); err != nil {
		t.Fatalf("Notify() = %v", err)
	}
}

func TestNotify_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Topic: "test", Priority: 3},
		client: srv.Client(),
	}

	err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test", Message: "hi"})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestTest_IncludesTagHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tags := r.Header.Get("Tags"); tags != "test" {
			t.Errorf("tags = %q, want test", tags)
		}
		if pri := r.Header.Get("Priority"); pri != "1" {
			t.Errorf("test priority = %q, want 1", pri)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Topic: "pilot", Priority: 3},
		client: srv.Client(),
	}

	if err := n.Test(context.Background()); err != nil {
		t.Fatalf("Test() = %v", err)
	}
}

func TestNew_DefaultPriority(t *testing.T) {
	n := New(Config{URL: "https://ntfy.sh", Topic: "test"})
	if n.cfg.Priority != 3 {
		t.Errorf("priority = %d, want 3", n.cfg.Priority)
	}
}

func TestNew_ClampsPriority(t *testing.T) {
	for _, p := range []int{0, -1, 6, 100} {
		n := New(Config{URL: "https://ntfy.sh", Topic: "test", Priority: p})
		if n.cfg.Priority != 3 {
			t.Errorf("priority(%d) = %d, want 3", p, n.cfg.Priority)
		}
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	n := New(Config{URL: "https://ntfy.sh/", Topic: "test"})
	if n.cfg.URL != "https://ntfy.sh" {
		t.Errorf("URL = %q, want no trailing slash", n.cfg.URL)
	}
}

func TestSanitizer(t *testing.T) {
	raw := json.RawMessage(`{"url":"https://ntfy.sh","topic":"pilot","token":"secret"}`)
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
	if result["topic"] != "pilot" {
		t.Errorf("topic was modified: %q", result["topic"])
	}
}
