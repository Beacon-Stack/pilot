// Package dbutil provides shared helpers for database-layer operations.
package dbutil

import (
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

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

// IsUniqueViolation reports whether err is a Postgres unique constraint
// violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
