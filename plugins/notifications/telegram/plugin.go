// Package telegram implements a Screenarr notification plugin that sends events
// to a Telegram chat via the Bot API.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/internal/safedialer"
	"github.com/screenarr/screenarr/pkg/plugin"
)

func init() {
	registry.Default.RegisterNotifier("telegram", func(settings json.RawMessage) (plugin.Notifier, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("telegram: invalid settings: %w", err)
		}
		if cfg.BotToken == "" {
			return nil, fmt.Errorf("telegram: bot_token is required")
		}
		if cfg.ChatID == "" {
			return nil, fmt.Errorf("telegram: chat_id is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterNotifierSanitizer("telegram", func(settings json.RawMessage) json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(settings, &m); err != nil {
			return json.RawMessage("{}")
		}
		if _, ok := m["bot_token"]; ok {
			m["bot_token"] = json.RawMessage(`"***"`)
		}
		out, _ := json.Marshal(m)
		return out
	})
}

// Config holds the user-supplied settings for a Telegram notifier.
type Config struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// Notifier is a Telegram notifier plugin instance.
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

func (n *Notifier) Name() string { return "Telegram" }

type sendMessagePayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// Notify sends the event as a Telegram message.
func (n *Notifier) Notify(ctx context.Context, event plugin.NotificationEvent) error {
	text := fmt.Sprintf("<b>[Screenarr] %s</b>\n%s", event.Type, event.Message)

	payload := sendMessagePayload{
		ChatID:    n.cfg.ChatID,
		Text:      text,
		ParseMode: "HTML",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshaling payload: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.cfg.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram: server returned %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// Test validates the bot token by calling getMe.
func (n *Notifier) Test(ctx context.Context) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", n.cfg.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("telegram: building test request: %w", err)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: test request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram: getMe returned %d — check bot_token", resp.StatusCode)
	}
	return nil
}
