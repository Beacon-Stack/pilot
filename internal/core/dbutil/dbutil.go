// Package dbutil provides shared helpers for database-layer operations.
package dbutil

import (
	"encoding/json"
	"errors"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// SQLite/sqlc note: with `emit_pointers_for_null_types: true`, sqlc-sqlite
// generates `*string` / `*int64` / `*time.Time` for nullable columns rather
// than `sql.NullXxx` structs. The helpers below operate on those pointer
// shapes directly so call sites do not have to mix types.

// NullableString returns nil for an empty string, otherwise a pointer to s.
// Used to feed sqlc-generated *string params from string values where the
// convention is "empty means SQL NULL".
func NullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// NullStringValue returns the dereferenced value of p, or "" if nil.
func NullStringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ParseRFC3339 parses an RFC3339(Nano) timestamp string, returning the zero
// time on any error.
func ParseRFC3339(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// FormatTime returns the RFC3339Nano UTC string for t. Used at write sites
// for TIMESTAMPTZ columns stored as TEXT.
func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// maskedSecret is the JSON value plugin sanitizers substitute for secrets.
const maskedSecret = `"***"`

// MergeSettings returns newSettings with any keys absent from newSettings
// filled in from existing. Keys present in newSettings win, except the
// masked placeholder "***": API responses mask secrets that way, so a
// client that echoes a response back must not overwrite the real secret.
// This preserves secret fields (passwords, API keys) when the frontend
// omits them, or round-trips them masked, in an update request.
func MergeSettings(existing, newSettings json.RawMessage) json.RawMessage {
	if len(newSettings) == 0 {
		return existing
	}
	var existingMap, newMap map[string]json.RawMessage
	if json.Unmarshal(existing, &existingMap) != nil {
		return newSettings
	}
	if json.Unmarshal(newSettings, &newMap) != nil {
		return newSettings
	}
	for k, v := range existingMap {
		nv, ok := newMap[k]
		if !ok || string(nv) == maskedSecret {
			newMap[k] = v
		}
	}
	merged, err := json.Marshal(newMap)
	if err != nil {
		return newSettings
	}
	return merged
}

// IsUniqueViolation reports whether err is a SQLite unique constraint violation
// (extended result codes SQLITE_CONSTRAINT_UNIQUE / SQLITE_CONSTRAINT_PRIMARYKEY).
func IsUniqueViolation(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return true
	}
	return false
}
