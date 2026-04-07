// Package pushover implements a Pilot notification plugin that sends events
// via the Pushover API.
package pushover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/internal/safedialer"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func init() {
	registry.Default.RegisterNotifier("pushover", func(settings json.RawMessage) (plugin.Notifier, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("pushover: invalid settings: %w", err)
		}
		if cfg.APIToken == "" {
			return nil, fmt.Errorf("pushover: api_token is required")
		}
		if cfg.UserKey == "" {
			return nil, fmt.Errorf("pushover: user_key is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterNotifierSanitizer("pushover", func(settings json.RawMessage) json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(settings, &m); err != nil {
			return json.RawMessage("{}")
		}
		if _, ok := m["api_token"]; ok {
			m["api_token"] = json.RawMessage(`"***"`)
		}
		out, _ := json.Marshal(m)
		return out
	})
}

// Config holds the user-supplied settings for a Pushover notifier.
type Config struct {
	APIToken string `json:"api_token"`
	UserKey  string `json:"user_key"`
}

// Notifier is a Pushover notifier plugin instance.
type Notifier struct {
	cfg    Config
	client *http.Client
}

// New creates a new Notifier from the given config.
func New(cfg Config) *Notifier {
	return &Notifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second, Transport: safedialer.Transport()},
	}
}

func (n *Notifier) Name() string { return "Pushover" }

type pushoverPayload struct {
	Token   string `json:"token"`
	User    string `json:"user"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// Notify sends the event as a Pushover notification.
func (n *Notifier) Notify(ctx context.Context, event plugin.NotificationEvent) error {
	payload := pushoverPayload{
		Token:   n.cfg.APIToken,
		User:    n.cfg.UserKey,
		Title:   fmt.Sprintf("Pilot — %s", event.Type),
		Message: event.Message,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pushover: marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.pushover.net/1/messages.json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("pushover: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("pushover: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("pushover: server returned %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// Test validates the API token and user key via the Pushover validate endpoint.
func (n *Notifier) Test(ctx context.Context) error {
	payload := map[string]string{
		"token": n.cfg.APIToken,
		"user":  n.cfg.UserKey,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pushover: marshaling test payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.pushover.net/1/users/validate.json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("pushover: building test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("pushover: test request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("pushover: validate returned %d: %s", resp.StatusCode, respBody)
	}
	return nil
}
