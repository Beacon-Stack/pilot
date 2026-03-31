// Package dbutil provides shared helpers for database-layer operations.
package dbutil

import "encoding/json"

// BoolToInt converts a bool to an int64 (1 or 0) for SQLite storage.
func BoolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// MergeSettings returns newSettings with any keys absent from newSettings
// filled in from existing. Keys present in newSettings always win.
// This preserves secret fields (passwords, API keys) when the frontend
// omits them from an update request.
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
