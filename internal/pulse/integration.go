// Package pulse integrates Prism with the Pulse control plane.
// When configured, Prism registers itself on startup, sends heartbeats,
// and can discover ecosystem services and shared configuration.
package pulse

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/beacon-stack/pulse/pkg/sdk"
	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/version"
)

// Integration wraps the Pulse SDK client and provides
// Prism-specific convenience methods.
type Integration struct {
	Client *sdk.Client
	logger *slog.Logger
}

// New creates and registers with Pulse. Returns nil (not an error)
// if Pulse is not configured — this makes integration optional.
//
// If only the URL is set (no API key), it will attempt to auto-discover
// the key from Pulse's config file at ~/.config/pulse/config.yaml.
func New(cfg config.PulseConfig, serverHost string, serverPort int, logger *slog.Logger) (*Integration, error) {
	if cfg.URL == "" {
		logger.Info("pulse integration disabled (no URL configured)")
		return nil, nil
	}

	apiKey := cfg.APIKey.Value()
	if apiKey == "" {
		// Try to auto-discover the API key from Pulse's local config.
		discovered := discoverAPIKey(logger)
		if discovered == "" {
			logger.Info("pulse integration disabled — no API key configured and could not auto-discover from ~/.config/pulse/config.yaml")
			return nil, nil
		}
		apiKey = discovered
		logger.Info("pulse: auto-discovered API key from local config file")
	}

	apiURL := fmt.Sprintf("http://%s:%d", serverHost, serverPort)
	healthURL := apiURL + "/health"

	// If the server host is 0.0.0.0, try the system hostname first
	// (works in Docker where the container name resolves), then fall
	// back to localhost (works for local dev).
	if serverHost == "0.0.0.0" || serverHost == "" {
		host := "localhost"
		if h, err := os.Hostname(); err == nil && h != "" {
			host = h
		}
		apiURL = fmt.Sprintf("http://%s:%d", host, serverPort)
		healthURL = apiURL + "/health"
	}

	client, err := sdk.New(sdk.Config{
		PulseURL: cfg.URL,
		APIKey:        apiKey,
		ServiceName:   "pilot",
		ServiceType:   "media-manager",
		APIURL:        apiURL,
		HealthURL:     healthURL,
		Version:       version.Version,
		Capabilities: []string{
			"supports_torrent",
			"supports_usenet",
			"supports_categories",
			"supports_tags",
			"supports_priorities",
			"content:tv",
		},
		Logger: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("pulse registration failed: %w", err)
	}

	return &Integration{Client: client, logger: logger}, nil
}

// Close stops heartbeats. Does not deregister — the service stays registered
// so Pulse can mark it as offline via health checks.
func (i *Integration) Close() {
	if i != nil && i.Client != nil {
		i.Client.Close()
	}
}

// DiscoverDownloadClients returns download clients registered in Pulse.
func (i *Integration) DiscoverDownloadClients(ctx context.Context) ([]sdk.Service, error) {
	return i.Client.DiscoverByType(ctx, "download-client")
}

// DiscoverIndexers returns indexers assigned to this service in Pulse.
func (i *Integration) DiscoverIndexers(ctx context.Context) ([]sdk.Indexer, error) {
	return i.Client.MyIndexers(ctx)
}

// GetQualityConfig returns all quality-related shared config.
func (i *Integration) GetQualityConfig(ctx context.Context) ([]sdk.ConfigEntry, error) {
	return i.Client.GetConfigNamespace(ctx, "quality")
}

// SubscribeToConfig subscribes to config namespace updates.
func (i *Integration) SubscribeToConfig(ctx context.Context, namespace string) error {
	return i.Client.Subscribe(ctx, namespace)
}

// discoverAPIKey attempts to read Pulse's API key from its local config file.
// This enables zero-config integration when both services run on the same machine.
func discoverAPIKey(logger *slog.Logger) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	cfgPath := fmt.Sprintf("%s/.config/pulse/config.yaml", home)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return ""
	}

	// Simple YAML key extraction — avoids importing a YAML parser just for this.
	// Looks for "api_key: " or 'api_key: "..."' in the file.
	for _, line := range splitLines(string(data)) {
		trimmed := trimLeftSpace(line)
		if hasPrefix(trimmed, "api_key:") {
			value := trimmed[len("api_key:"):]
			value = trimSpace(value)
			value = trimQuotes(value)
			if value != "" && value != "***" {
				return value
			}
		}
	}

	return ""
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimLeftSpace(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[i:]
		}
	}
	return ""
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func trimQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
