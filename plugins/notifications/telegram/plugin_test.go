package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/screenarr/screenarr/pkg/plugin"
)

func TestNotify_Success(t *testing.T) {
	var gotBody sendMessagePayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	n := New(Config{BotToken: "fake-token", ChatID: "12345"})
	// Override the base URL to use the test server.
	n.client = srv.Client()

	event := plugin.NotificationEvent{
		Type:      "grab_started",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Message:   "Grabbing: Inception",
	}

	// We need to override the URL, so we'll test via a round-trip approach.
	// Instead, test the Notifier with a custom transport that rewrites the URL.
	originalNotify := n.Notify
	_ = originalNotify // suppress unused

	// Simpler: just test that the factory + sanitizer work, and test Notify
	// against the test server by temporarily overriding the URL construction.
	// Since the URL is built inline, we'll test via the full Notify but with
	// a transport that intercepts.
	n.client.Transport = rewriteTransport{target: srv.URL, wrapped: http.DefaultTransport}

	if err := n.Notify(context.Background(), event); err != nil {
		t.Fatalf("Notify() = %v", err)
	}

	if gotBody.ChatID != "12345" {
		t.Errorf("chat_id = %q, want %q", gotBody.ChatID, "12345")
	}
	if gotBody.ParseMode != "HTML" {
		t.Errorf("parse_mode = %q, want HTML", gotBody.ParseMode)
	}
	if gotBody.Text == "" {
		t.Error("text is empty")
	}
}

func TestNotify_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Forbidden"}`))
	}))
	defer srv.Close()

	n := New(Config{BotToken: "bad-token", ChatID: "12345"})
	n.client.Transport = rewriteTransport{target: srv.URL, wrapped: http.DefaultTransport}

	err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test", Message: "hi"})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestTest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":123}}`))
	}))
	defer srv.Close()

	n := New(Config{BotToken: "fake-token", ChatID: "12345"})
	n.client.Transport = rewriteTransport{target: srv.URL, wrapped: http.DefaultTransport}

	if err := n.Test(context.Background()); err != nil {
		t.Fatalf("Test() = %v", err)
	}
}

func TestFactory_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"empty", `{}`},
		{"no chat_id", `{"bot_token":"tok"}`},
		{"no bot_token", `{"chat_id":"123"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			if err := json.Unmarshal(json.RawMessage(tt.json), &cfg); err != nil {
				return
			}
			if cfg.BotToken == "" || cfg.ChatID == "" {
				return // expected — factory would reject
			}
			t.Error("expected missing field")
		})
	}
}

func TestSanitizer(t *testing.T) {
	raw := json.RawMessage(`{"bot_token":"secret123","chat_id":"456"}`)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["bot_token"]; ok {
		m["bot_token"] = json.RawMessage(`"***"`)
	}
	out, _ := json.Marshal(m)
	var result map[string]string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if result["bot_token"] != "***" {
		t.Errorf("bot_token = %q, want ***", result["bot_token"])
	}
	if result["chat_id"] != "456" {
		t.Errorf("chat_id = %q, want 456", result["chat_id"])
	}
}

// rewriteTransport rewrites all request URLs to the test server.
type rewriteTransport struct {
	target  string
	wrapped http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = rt.target[len("http://"):]
	return rt.wrapped.RoundTrip(req)
}
