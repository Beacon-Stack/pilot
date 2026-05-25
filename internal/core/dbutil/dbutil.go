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

// NullStringFromString is an alias of NullableString.
func NullStringFromString(s string) *string {
	return NullableString(s)
}

// NullString returns p unchanged. Kept as a no-op for callers that built
// against the older sql.NullString-based API.
func NullString(p *string) *string {
	return p
}

// NullStringPtr is the identity for *string.
func NullStringPtr(p *string) *string {
	return p
}

// NullStringValue returns the dereferenced value of p, or "" if nil.
func NullStringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// NullInt64FromInt converts a Go int to *int64. Returns nil for zero values
// when callers want "0 means NULL"; otherwise pair with a direct &v cast.
func NullInt64FromInt(v int) *int64 {
	i := int64(v)
	return &i
}

// NullInt32 converts a *int to *int64 (SQLite stores INTEGER as int64).
// A nil pointer yields nil.
func NullInt32(p *int) *int64 {
	if p == nil {
		return nil
	}
	v := int64(*p)
	return &v
}

// NullInt32FromInt64Ptr returns p unchanged; sqlc-sqlite emits *int64 for
// nullable integer columns. Kept under the old name so existing call sites
// continue to compile against the migrated API.
func NullInt32FromInt64Ptr(p *int64) *int64 {
	return p
}

// NullInt32Value returns the dereferenced value of p, or 0 if nil.
func NullInt32Value(p *int64) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

// NullTime converts a *time.Time to its RFC3339Nano UTC string representation.
// A nil pointer yields nil so the column stores as SQL NULL.
//
// TIMESTAMPTZ columns are stored as TEXT in SQLite (see migration squash);
// using RFC3339 keeps lexicographic ordering identical to chronological
// ordering, which several queries rely on.
func NullTime(p *time.Time) *string {
	if p == nil {
		return nil
	}
	s := p.UTC().Format(time.RFC3339Nano)
	return &s
}

// NullTimePtr parses an RFC3339-encoded *string back to *time.Time. Nil or
// unparseable input yields nil.
func NullTimePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, *s)
		if err != nil {
			return nil
		}
	}
	return &t
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

// MergeSettings returns newSettings with any keys absent from newSettings
// filled in from existing. Keys present in newSettings always win.
// This is used to preserve secret fields (passwords, API keys) when the
// frontend omits them from an update request.
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
		if _, ok := newMap[k]; !ok {
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
