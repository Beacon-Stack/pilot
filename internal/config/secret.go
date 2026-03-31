package config

import "log/slog"

// Secret is a string type that never reveals its contents when logged,
// printed, or serialized. Use it for API keys, passwords, and tokens.
//
// Call .Value() when you actually need the underlying string (e.g. to
// make an authenticated HTTP request).
type Secret string

// String implements fmt.Stringer. Always returns "***".
func (s Secret) String() string { return "***" }

// GoString implements fmt.GoStringer. Always returns "***".
func (s Secret) GoString() string { return "***" }

// MarshalJSON always serializes as the string "***".
// Prevents accidental exposure in JSON API responses.
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"***"`), nil }

// MarshalText always encodes as "***".
// Covers YAML, TOML, and other text-based serializers.
func (s Secret) MarshalText() ([]byte, error) { return []byte("***"), nil }

// LogValue implements slog.LogValuer. Always returns "***".
// Prevents exposure in structured log output.
func (s Secret) LogValue() slog.Value { return slog.StringValue("***") }

// Value returns the underlying secret string.
// Only call this when the actual value is needed.
func (s Secret) Value() string { return string(s) }

// IsEmpty reports whether the secret has no value.
func (s Secret) IsEmpty() bool { return s == "" }
