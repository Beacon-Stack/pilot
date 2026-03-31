package discord

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
	var gotPayload discordPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{WebhookURL: srv.URL, Username: "TestBot"},
		client: srv.Client(),
	}

	event := plugin.NotificationEvent{
		Type:      plugin.EventGrabStarted,
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Message:   "Grabbed: Inception",
	}

	if err := n.Notify(context.Background(), event); err != nil {
		t.Fatalf("Notify() = %v", err)
	}

	if gotPayload.Username != "TestBot" {
		t.Errorf("username = %q, want TestBot", gotPayload.Username)
	}
	if len(gotPayload.Embeds) != 1 {
		t.Fatalf("embeds len = %d, want 1", len(gotPayload.Embeds))
	}
	if gotPayload.Embeds[0].Description != "Grabbed: Inception" {
		t.Errorf("description = %q", gotPayload.Embeds[0].Description)
	}
}

func TestNotify_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{WebhookURL: srv.URL},
		client: srv.Client(),
	}

	err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test", Message: "hi"})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

func TestNew_DefaultUsername(t *testing.T) {
	n := New(Config{WebhookURL: "http://example.com"})
	if n.cfg.Username != "Screenarr" {
		t.Errorf("Username = %q, want Screenarr", n.cfg.Username)
	}
}

func TestColorForEvent(t *testing.T) {
	tests := []struct {
		event plugin.EventType
		color int
	}{
		{plugin.EventGrabStarted, 0x5865F2},
		{plugin.EventDownloadDone, 0x57F287},
		{plugin.EventImportDone, 0x57F287},
		{plugin.EventImportFailed, 0xED4245},
		{plugin.EventHealthIssue, 0xED4245},
		{plugin.EventHealthOK, 0x57F287},
		{plugin.EventSeriesAdded, 0xFEE75C},
		{plugin.EventType("unknown"), 0x5865F2},
	}
	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			got := colorForEvent(tt.event)
			if got != tt.color {
				t.Errorf("colorForEvent(%s) = %#x, want %#x", tt.event, got, tt.color)
			}
		})
	}
}
