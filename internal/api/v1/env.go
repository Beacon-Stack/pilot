package v1

import (
	"context"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// envPrefixes defines which env vars surface in /api/v1/system/env.
// Keep this narrow — random shell/system vars like PATH, HOSTNAME,
// HOME aren't useful in the dashboard and just clutter the panel.
var envPrefixes = []string{"PILOT_", "TZ", "LANG", "LC_"}

// envRedactSubstrings — any env key containing these (case-insensitive)
// has its value replaced with "[redacted]". Belt-and-braces for the cases
// where someone names a config knob PILOT_FOO_API_KEY etc.
var envRedactSubstrings = []string{
	"PASSWORD", "SECRET", "KEY", "TOKEN", "DSN", "AUTH",
}

type envEntryBody struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Redacted bool   `json:"redacted"`
}

type envOutput struct {
	Body []envEntryBody
}

// RegisterEnvRoutes exposes the relevant env vars for this process so the
// Pulse dashboard can show what the operator configured. Secrets are
// redacted by substring match on the key.
func RegisterEnvRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "system-env",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/env",
		Summary:     "Relevant environment variables (secrets redacted)",
		Tags:        []string{"System"},
	}, func(_ context.Context, _ *struct{}) (*envOutput, error) {
		out := collectEnv()
		return &envOutput{Body: out}, nil
	})
}

func collectEnv() []envEntryBody {
	all := os.Environ()
	out := make([]envEntryBody, 0, 16)
	for _, kv := range all {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		key, val := kv[:i], kv[i+1:]
		if !envKeyAllowed(key) {
			continue
		}
		entry := envEntryBody{Key: key, Value: val}
		if envKeyShouldRedact(key) {
			entry.Value = "[redacted]"
			entry.Redacted = true
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func envKeyAllowed(key string) bool {
	for _, p := range envPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

func envKeyShouldRedact(key string) bool {
	upper := strings.ToUpper(key)
	for _, sub := range envRedactSubstrings {
		if strings.Contains(upper, sub) {
			return true
		}
	}
	return false
}
