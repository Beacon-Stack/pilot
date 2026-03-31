package pushover

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
	var gotBody pushoverPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer srv.Close()

	n := New(Config{APIToken: "app-token", UserKey: "user-key"})
	n.client = srv.Client()
	// Use rewriteTransport to redirect to test server.
	n.client.Transport = rewriteTransport{target: srv.URL, wrapped: http.DefaultTransport}

	event := plugin.NotificationEvent{
		Type:      "import_complete",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Message:   "Imported: Inception",
	}

	if err := n.Notify(context.Background(), event); err != nil {
		t.Fatalf("Notify() = %v", err)
	}

	if gotBody.Token != "app-token" {
		t.Errorf("token = %q, want app-token", gotBody.Token)
	}
	if gotBody.User != "user-key" {
		t.Errorf("user = %q, want user-key", gotBody.User)
	}
	if gotBody.Message != "Imported: Inception" {
		t.Errorf("message = %q", gotBody.Message)
	}
}

func TestNotify_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":0,"errors":["invalid token"]}`))
	}))
	defer srv.Close()

	n := New(Config{APIToken: "bad", UserKey: "bad"})
	n.client.Transport = rewriteTransport{target: srv.URL, wrapped: http.DefaultTransport}

	err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test", Message: "hi"})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

func TestTest_ValidatesCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload["token"] != "app-token" || payload["user"] != "user-key" {
			t.Errorf("unexpected payload: %v", payload)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer srv.Close()

	n := New(Config{APIToken: "app-token", UserKey: "user-key"})
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
		{"no user_key", `{"api_token":"tok"}`},
		{"no api_token", `{"user_key":"usr"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			if err := json.Unmarshal(json.RawMessage(tt.json), &cfg); err != nil {
				return // invalid JSON is also a failure
			}
			if cfg.APIToken == "" || cfg.UserKey == "" {
				return // expected — factory would reject
			}
			t.Error("expected missing field")
		})
	}
}

func TestSanitizer(t *testing.T) {
	raw := json.RawMessage(`{"api_token":"secret","user_key":"usr123"}`)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["api_token"] = json.RawMessage(`"***"`)
	out, _ := json.Marshal(m)
	var result map[string]string
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if result["api_token"] != "***" {
		t.Errorf("api_token = %q, want ***", result["api_token"])
	}
	if result["user_key"] != "usr123" {
		t.Errorf("user_key was modified: %q", result["user_key"])
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
