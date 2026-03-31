// Package dbutil provides shared helpers for database-layer operations.
package dbutil

import (
	"encoding/json"
	"errors"

	sqlitedrv "modernc.org/sqlite"
)

// BoolToInt converts a bool to an int64 (1 or 0) for SQLite storage.
func BoolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
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

// IsUniqueViolation reports whether err is a SQLite unique constraint violation.
// Uses the driver's error code (2067 = SQLITE_CONSTRAINT_UNIQUE) rather than
// fragile string matching.
func IsUniqueViolation(err error) bool {
	var e *sqlitedrv.Error
	if errors.As(err, &e) {
		const sqliteConstraintUnique = 2067
		return e.Code() == sqliteConstraintUnique
	}
	return false
}
