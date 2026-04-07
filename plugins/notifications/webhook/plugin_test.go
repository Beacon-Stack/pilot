package webhook

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
	var gotEvent plugin.NotificationEvent
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		gotHeaders = r.Header
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotEvent); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg: Config{
			URL:     srv.URL,
			Method:  http.MethodPost,
			Headers: map[string]string{"X-Custom": "test-value"},
		},
		client: srv.Client(),
	}

	event := plugin.NotificationEvent{
		Type:      plugin.EventDownloadDone,
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Message:   "Download complete: Inception",
	}

	if err := n.Notify(context.Background(), event); err != nil {
		t.Fatalf("Notify() = %v", err)
	}

	if gotEvent.Message != "Download complete: Inception" {
		t.Errorf("message = %q", gotEvent.Message)
	}
	if gotHeaders.Get("X-Custom") != "test-value" {
		t.Errorf("X-Custom header = %q, want test-value", gotHeaders.Get("X-Custom"))
	}
	if gotHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotHeaders.Get("Content-Type"))
	}
}

func TestNotify_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Method: http.MethodPost},
		client: srv.Client(),
	}

	err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test", Message: "hi"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestNew_DefaultMethod(t *testing.T) {
	n := New(Config{URL: "http://example.com"})
	if n.cfg.Method != http.MethodPost {
		t.Errorf("Method = %q, want POST", n.cfg.Method)
	}
}

func TestNotify_CustomMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Method: http.MethodPut},
		client: srv.Client(),
	}

	if err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test", Message: "hi"}); err != nil {
		t.Fatalf("Notify() = %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
}

func TestName(t *testing.T) {
	n := New(Config{URL: "http://example.com"})
	if n.Name() != "Webhook" {
		t.Errorf("Name() = %q, want %q", n.Name(), "Webhook")
	}
}

func TestTest_SendsTestEvent(t *testing.T) {
	var gotEvent plugin.NotificationEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotEvent)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{URL: srv.URL, Method: http.MethodPost},
		client: srv.Client(),
	}

	if err := n.Test(context.Background()); err != nil {
		t.Fatalf("Test() = %v", err)
	}
	if gotEvent.Type != "test" {
		t.Errorf("event type = %q, want %q", gotEvent.Type, "test")
	}
	if gotEvent.Message == "" {
		t.Error("event message should not be empty")
	}
}

func TestNew_CustomMethod(t *testing.T) {
	n := New(Config{URL: "http://example.com", Method: http.MethodPut})
	if n.cfg.Method != http.MethodPut {
		t.Errorf("Method = %q, want PUT", n.cfg.Method)
	}
}

func TestNew_ClientNotNil(t *testing.T) {
	n := New(Config{URL: "http://example.com"})
	if n.client == nil {
		t.Fatal("client should not be nil")
	}
}

func TestNotify_MultipleHeaders(t *testing.T) {
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg: Config{
			URL:    srv.URL,
			Method: http.MethodPost,
			Headers: map[string]string{
				"Authorization": "Bearer abc",
				"X-Custom":      "value",
			},
		},
		client: srv.Client(),
	}

	if err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test"}); err != nil {
		t.Fatal(err)
	}
	if gotHeaders.Get("Authorization") != "Bearer abc" {
		t.Errorf("Authorization = %q, want 'Bearer abc'", gotHeaders.Get("Authorization"))
	}
	if gotHeaders.Get("X-Custom") != "value" {
		t.Errorf("X-Custom = %q, want 'value'", gotHeaders.Get("X-Custom"))
	}
}
