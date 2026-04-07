package slack

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
	var gotPayload slackPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{WebhookURL: srv.URL, Username: "PilotBot", IconEmoji: ":movie_camera:"},
		client: srv.Client(),
	}

	event := plugin.NotificationEvent{
		Type:      plugin.EventImportDone,
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Message:   "Imported: Inception",
	}

	if err := n.Notify(context.Background(), event); err != nil {
		t.Fatalf("Notify() = %v", err)
	}

	if gotPayload.Username != "PilotBot" {
		t.Errorf("username = %q, want PilotBot", gotPayload.Username)
	}
	if len(gotPayload.Attachments) != 1 {
		t.Fatalf("attachments len = %d, want 1", len(gotPayload.Attachments))
	}
	if gotPayload.Attachments[0].Text != "Imported: Inception" {
		t.Errorf("text = %q", gotPayload.Attachments[0].Text)
	}
}

func TestNotify_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	n := &Notifier{
		cfg:    Config{WebhookURL: srv.URL},
		client: srv.Client(),
	}

	err := n.Notify(context.Background(), plugin.NotificationEvent{Type: "test", Message: "hi"})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestNew_Defaults(t *testing.T) {
	n := New(Config{WebhookURL: "http://example.com"})
	if n.cfg.Username != "Pilot" {
		t.Errorf("Username = %q, want Pilot", n.cfg.Username)
	}
	if n.cfg.IconEmoji != ":clapper:" {
		t.Errorf("IconEmoji = %q, want :clapper:", n.cfg.IconEmoji)
	}
}

func TestColorForEvent(t *testing.T) {
	tests := []struct {
		event plugin.EventType
		color string
	}{
		{plugin.EventGrabStarted, "#5865F2"},
		{plugin.EventDownloadDone, "good"},
		{plugin.EventImportFailed, "danger"},
		{plugin.EventSeriesAdded, "warning"},
		{plugin.EventType("unknown"), "#5865F2"},
	}
	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			got := colorForEvent(tt.event)
			if got != tt.color {
				t.Errorf("colorForEvent(%s) = %q, want %q", tt.event, got, tt.color)
			}
		})
	}
}
