// Package gotify implements a Screenarr notification plugin that sends events
// to a Gotify server via its REST API.
package gotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/internal/safedialer"
	"github.com/screenarr/screenarr/pkg/plugin"
)

func init() {
	registry.Default.RegisterNotifier("gotify", func(settings json.RawMessage) (plugin.Notifier, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("gotify: invalid settings: %w", err)
		}
		if cfg.URL == "" {
			return nil, fmt.Errorf("gotify: url is required")
		}
		if cfg.Token == "" {
			return nil, fmt.Errorf("gotify: token is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterNotifierSanitizer("gotify", func(settings json.RawMessage) json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(settings, &m); err != nil {
			return json.RawMessage("{}")
		}
		if _, ok := m["token"]; ok {
			m["token"] = json.RawMessage(`"***"`)
		}
		out, _ := json.Marshal(m)
		return out
	})
}

// Config holds the user-supplied settings for a Gotify notifier.
type Config struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// Notifier is a Gotify notifier plugin instance.
type Notifier struct {
	cfg    Config
	client *http.Client
}

// New creates a new Notifier from the given config.
func New(cfg Config) *Notifier {
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	return &Notifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second, Transport: safedialer.LANTransport()},
	}
}

func (n *Notifier) Name() string { return "Gotify" }

type gotifyPayload struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

// Notify sends the event as a Gotify message.
func (n *Notifier) Notify(ctx context.Context, event plugin.NotificationEvent) error {
	payload := gotifyPayload{
		Title:    fmt.Sprintf("Screenarr — %s", event.Type),
		Message:  event.Message,
		Priority: 5,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("gotify: marshaling payload: %w", err)
	}

	url := fmt.Sprintf("%s/message?token=%s", n.cfg.URL, n.cfg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("gotify: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("gotify: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("gotify: server returned %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// Test sends a low-priority test message to verify the Gotify server is reachable.
func (n *Notifier) Test(ctx context.Context) error {
	payload := gotifyPayload{
		Title:    "Screenarr",
		Message:  "Screenarr Gotify test — connection successful",
		Priority: 1,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("gotify: marshaling test payload: %w", err)
	}

	url := fmt.Sprintf("%s/message?token=%s", n.cfg.URL, n.cfg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("gotify: building test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("gotify: test request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("gotify: test returned %d: %s", resp.StatusCode, respBody)
	}
	return nil
}
