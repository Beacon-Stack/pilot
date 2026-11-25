package availability

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

const displaySettingKey = "display"

// Display holds the choices about how series progress is shown.
type Display struct {
	// CountSpecials includes season 0 in a series' episode counts and
	// its state. Off by default: most people don't collect specials, and
	// counting them makes a complete show look unfinished.
	CountSpecials bool `json:"count_specials"`
}

// DisplayStore is the part of the database the display settings need.
type DisplayStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, arg db.SetSettingParams) error
}

// DisplaySettings reads and saves Display.
type DisplaySettings struct {
	q DisplayStore
}

// NewDisplaySettings creates a DisplaySettings.
func NewDisplaySettings(q DisplayStore) *DisplaySettings {
	return &DisplaySettings{q: q}
}

// Get returns the saved settings, or the defaults when none are saved.
func (d *DisplaySettings) Get(ctx context.Context) (Display, error) {
	raw, err := d.q.GetSetting(ctx, displaySettingKey)
	if errors.Is(err, sql.ErrNoRows) {
		return Display{}, nil
	}
	if err != nil {
		return Display{}, err
	}
	var out Display
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return Display{}, fmt.Errorf("decoding display settings: %w", err)
	}
	return out, nil
}

// Set saves the settings.
func (d *DisplaySettings) Set(ctx context.Context, v Display) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return d.q.SetSetting(ctx, db.SetSettingParams{
		Key:       displaySettingKey,
		Value:     string(b),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}
