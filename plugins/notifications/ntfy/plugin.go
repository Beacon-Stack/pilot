// Package ntfy implements a Pilot notification plugin that sends events
// to an ntfy server (ntfy.sh or self-hosted).
package ntfy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/beacon-media/pilot/internal/registry"
	"github.com/beacon-media/pilot/internal/safedialer"
	"github.com/beacon-media/pilot/pkg/plugin"
)

func init() {
	registry.Default.RegisterNotifier("ntfy", func(settings json.RawMessage) (plugin.Notifier, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("ntfy: invalid settings: %w", err)
		}
		if cfg.URL == "" {
			return nil, fmt.Errorf("ntfy: url is required")
		}
		if cfg.Topic == "" {
			return nil, fmt.Errorf("ntfy: topic is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterNotifierSanitizer("ntfy", func(settings json.RawMessage) json.RawMessage {
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

// Config holds the user-supplied settings for an ntfy notifier.
type Config struct {
	URL      string `json:"url"`
	Topic    string `json:"topic"`
	Token    string `json:"token,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// Notifier is an ntfy notifier plugin instance.
type Notifier struct {
	cfg    Config
	client *http.Client
}

// New creates a new Notifier from the given config.
func New(cfg Config) *Notifier {
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	if cfg.Priority < 1 || cfg.Priority > 5 {
		cfg.Priority = 3
	}
	return &Notifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second, Transport: safedialer.LANTransport()},
	}
}

func (n *Notifier) Name() string { return "ntfy" }

// Notify sends the event as an ntfy message.
func (n *Notifier) Notify(ctx context.Context, event plugin.NotificationEvent) error {
	url := fmt.Sprintf("%s/%s", n.cfg.URL, n.cfg.Topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(event.Message))
	if err != nil {
		return fmt.Errorf("ntfy: building request: %w", err)
	}

	req.Header.Set("Title", fmt.Sprintf("Pilot — %s", event.Type))
	req.Header.Set("Priority", fmt.Sprintf("%d", n.cfg.Priority))
	if n.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.cfg.Token)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ntfy: server returned %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// Test sends a test message with the "test" tag.
func (n *Notifier) Test(ctx context.Context) error {
	url := fmt.Sprintf("%s/%s", n.cfg.URL, n.cfg.Topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		strings.NewReader("Pilot ntfy test — connection successful"))
	if err != nil {
		return fmt.Errorf("ntfy: building test request: %w", err)
	}

	req.Header.Set("Title", "Pilot")
	req.Header.Set("Tags", "test")
	req.Header.Set("Priority", "1")
	if n.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.cfg.Token)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy: test request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ntfy: test returned %d: %s", resp.StatusCode, respBody)
	}
	return nil
}
