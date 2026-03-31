// Package webhook implements a Screenarr notification plugin that sends events
// as JSON HTTP POST requests to a user-configured URL.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/internal/safedialer"
	"github.com/screenarr/screenarr/pkg/plugin"
)

func init() {
	registry.Default.RegisterNotifier("webhook", func(settings json.RawMessage) (plugin.Notifier, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("webhook: invalid settings: %w", err)
		}
		if cfg.URL == "" {
			return nil, fmt.Errorf("webhook: url is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterNotifierSanitizer("webhook", func(settings json.RawMessage) json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(settings, &m); err != nil {
			return json.RawMessage("{}")
		}
		// Redact all header values — they may contain Authorization tokens.
		if raw, ok := m["headers"]; ok {
			var headers map[string]string
			if err := json.Unmarshal(raw, &headers); err == nil {
				redacted := make(map[string]string, len(headers))
				for k := range headers {
					redacted[k] = "***"
				}
				if b, err := json.Marshal(redacted); err == nil {
					m["headers"] = b
				}
			}
		}
		out, _ := json.Marshal(m)
		return out
	})
}

// Config holds the user-supplied settings for a webhook notifier.
type Config struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`  // default: POST
	Headers map[string]string `json:"headers,omitempty"` // extra request headers
}

// Notifier is a webhook notifier plugin instance.
type Notifier struct {
	cfg    Config
	client *http.Client
}

// New creates a new Notifier from the given config.
func New(cfg Config) *Notifier {
	if cfg.Method == "" {
		cfg.Method = http.MethodPost
	}
	return &Notifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second, Transport: safedialer.LANTransport()},
	}
}

func (n *Notifier) Name() string { return "Webhook" }

// Notify sends the event as a JSON payload to the configured URL.
func (n *Notifier) Notify(ctx context.Context, event plugin.NotificationEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("webhook: marshaling event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, n.cfg.Method, n.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range n.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: server returned %d", resp.StatusCode)
	}
	return nil
}

// Test sends a test event to verify the webhook is reachable.
func (n *Notifier) Test(ctx context.Context) error {
	return n.Notify(ctx, plugin.NotificationEvent{
		Type:      plugin.EventType("test"),
		Timestamp: time.Now().UTC(),
		Message:   "Screenarr webhook test — connection successful",
	})
}
