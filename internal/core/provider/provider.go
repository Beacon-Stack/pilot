// Package provider resolves effective third-party API keys at lookup
// time. Precedence, first match wins:
//
//  1. DB override (settings table, set via the Settings UI)
//  2. Baked-in default from config.DefaultTMDBKey / DefaultTraktClientID
//  3. Empty string — callers should treat this as "no key configured"
//     and return a 503-level error.
//
// The DB-override mechanism lets operators rotate a leaked or rate-
// limited baked key without rebuilding the image. The baked default
// is never returned through any API; the Settings UI shows only a
// redacted preview for overrides.
package provider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beacon-stack/pilot/internal/config"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// Provider identifiers — used as DB keys in the settings table and as
// API path segments.
const (
	TVDB  = "tvdb"
	Trakt = "trakt"
)

// Source reports where an effective key came from.
type Source string

const (
	// SourceOverride — the DB holds a user-set value.
	SourceOverride Source = "override"
	// SourceDefault — no override; binary default (possibly empty).
	SourceDefault Source = "default"
)

// Store is the subset of the DB queries interface this package needs.
// Accepts *db.Queries and any handwritten implementation that matches.
type Store interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, arg db.SetSettingParams) error
	DeleteSetting(ctx context.Context, key string) error
}

// Resolver reads and writes provider API key overrides in the settings
// table and falls back to the ldflag-baked defaults when no override
// is set.
type Resolver struct {
	store Store
}

// NewResolver constructs a Resolver backed by the given settings store.
func NewResolver(store Store) *Resolver {
	return &Resolver{store: store}
}

// EffectiveKey returns the key that should actually be used for API
// calls to the named provider. Empty string means "not configured" —
// callers should return a 503 to the caller with a helpful message.
func (r *Resolver) EffectiveKey(ctx context.Context, name string) (string, Source, error) {
	override, err := r.override(ctx, name)
	if err != nil {
		return "", "", err
	}
	if override != "" {
		return override, SourceOverride, nil
	}
	return bakedDefault(name), SourceDefault, nil
}

// Preview returns a redacted representation of the effective key for
// Settings-page display: "•••..." with the last 3 characters revealed.
// An empty effective key becomes "not configured".
func (r *Resolver) Preview(ctx context.Context, name string) (string, Source, error) {
	value, source, err := r.EffectiveKey(ctx, name)
	if err != nil {
		return "", "", err
	}
	return redact(value), source, nil
}

// SetOverride persists a user-supplied key as the override for the
// named provider. Trims surrounding whitespace; rejects empty values
// (use ClearOverride to revert to the baked default).
func (r *Resolver) SetOverride(ctx context.Context, name, value string) error {
	if !isKnown(name) {
		return fmt.Errorf("unknown provider %q", name)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("override value cannot be empty; use ClearOverride to revert to the baked default")
	}
	return r.store.SetSetting(ctx, db.SetSettingParams{
		Key:       overrideKey(name),
		Value:     value,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// ClearOverride removes any stored override for the named provider,
// so the next lookup falls back to the baked default.
func (r *Resolver) ClearOverride(ctx context.Context, name string) error {
	if !isKnown(name) {
		return fmt.Errorf("unknown provider %q", name)
	}
	return r.store.DeleteSetting(ctx, overrideKey(name))
}

// HasOverride reports whether an override is currently stored for the
// named provider (separate from whether a baked default exists).
func (r *Resolver) HasOverride(ctx context.Context, name string) (bool, error) {
	v, err := r.override(ctx, name)
	if err != nil {
		return false, err
	}
	return v != "", nil
}

func (r *Resolver) override(ctx context.Context, name string) (string, error) {
	if !isKnown(name) {
		return "", fmt.Errorf("unknown provider %q", name)
	}
	v, err := r.store.GetSetting(ctx, overrideKey(name))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("reading override for %s: %w", name, err)
	}
	return v, nil
}

func overrideKey(name string) string { return "provider." + name + ".api_key" }

func isKnown(name string) bool {
	switch name {
	case TVDB, Trakt:
		return true
	}
	return false
}

func bakedDefault(name string) string {
	switch name {
	case TVDB:
		return config.DefaultTMDBKey()
	case Trakt:
		return config.DefaultTraktClientID()
	}
	return ""
}

// redact keeps the last 3 chars of a key and replaces the rest with
// bullets. Short keys (<= 3 chars) become fully bulleted to avoid
// leaking the whole thing.
func redact(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 3 {
		return strings.Repeat("•", len(s))
	}
	return strings.Repeat("•", len(s)-3) + s[len(s)-3:]
}
