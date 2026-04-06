// Package mediaserver manages media server configurations (Plex, Emby, Jellyfin).
package mediaserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-media/pilot/internal/core/dbutil"
	dbsqlite "github.com/beacon-media/pilot/internal/db/generated/sqlite"
	"github.com/beacon-media/pilot/internal/registry"
)

// ErrNotFound is returned when a media server config does not exist.
var ErrNotFound = errors.New("media server not found")

// Config is the domain representation of a stored media server configuration.
type Config struct {
	ID        string
	Name      string
	Kind      string // "plex", "emby", "jellyfin"
	Enabled   bool
	Settings  json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateRequest carries the fields needed to create a media server config.
type CreateRequest struct {
	Name     string
	Kind     string
	Enabled  bool
	Settings json.RawMessage
}

// UpdateRequest carries the fields needed to update a media server config.
type UpdateRequest = CreateRequest

// Service manages media server configurations.
type Service struct {
	q   dbsqlite.Querier
	reg *registry.Registry
}

// NewService creates a new Service.
func NewService(q dbsqlite.Querier, reg *registry.Registry) *Service {
	return &Service{q: q, reg: reg}
}

// Create persists a new media server configuration.
func (s *Service) Create(ctx context.Context, req CreateRequest) (Config, error) {
	settings := req.Settings
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}
	if _, err := s.reg.NewMediaServer(req.Kind, settings); err != nil {
		return Config{}, fmt.Errorf("invalid media server kind or settings: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateMediaServerConfig(ctx, dbsqlite.CreateMediaServerConfigParams{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Kind:      req.Kind,
		Enabled:   dbutil.BoolToInt(req.Enabled),
		Settings:  string(settings),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return Config{}, fmt.Errorf("inserting media server config: %w", err)
	}
	return rowToConfig(row), nil
}

// Get returns a media server config by ID. Returns ErrNotFound if absent.
func (s *Service) Get(ctx context.Context, id string) (Config, error) {
	row, err := s.q.GetMediaServerConfig(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("fetching media server %q: %w", id, err)
	}
	return rowToConfig(row), nil
}

// List returns all media server configs ordered by name.
func (s *Service) List(ctx context.Context) ([]Config, error) {
	rows, err := s.q.ListMediaServerConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing media server configs: %w", err)
	}
	configs := make([]Config, 0, len(rows))
	for _, row := range rows {
		configs = append(configs, rowToConfig(row))
	}
	return configs, nil
}

// Update replaces the mutable fields of a media server config.
// Returns ErrNotFound if the config does not exist.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Config, error) {
	if _, err := s.q.GetMediaServerConfig(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("fetching media server %q for update: %w", id, err)
	}

	settings := req.Settings
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}

	row, err := s.q.UpdateMediaServerConfig(ctx, dbsqlite.UpdateMediaServerConfigParams{
		ID:        id,
		Name:      req.Name,
		Kind:      req.Kind,
		Enabled:   dbutil.BoolToInt(req.Enabled),
		Settings:  string(settings),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return Config{}, fmt.Errorf("updating media server %q: %w", id, err)
	}
	return rowToConfig(row), nil
}

// Delete removes a media server config. Returns ErrNotFound if absent.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.q.GetMediaServerConfig(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("fetching media server %q for delete: %w", id, err)
	}
	if err := s.q.DeleteMediaServerConfig(ctx, id); err != nil {
		return fmt.Errorf("deleting media server %q: %w", id, err)
	}
	return nil
}

// Test instantiates the media server plugin and verifies connectivity.
// Returns ErrNotFound if the config does not exist.
func (s *Service) Test(ctx context.Context, id string) error {
	cfg, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	ms, err := s.reg.NewMediaServer(cfg.Kind, cfg.Settings)
	if err != nil {
		return fmt.Errorf("instantiating media server plugin: %w", err)
	}
	return ms.Test(ctx)
}

// rowToConfig converts a DB row into the domain Config type.
func rowToConfig(row dbsqlite.MediaServerConfig) Config {
	createdAt, _ := time.Parse(time.RFC3339, row.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, row.UpdatedAt)

	return Config{
		ID:        row.ID,
		Name:      row.Name,
		Kind:      row.Kind,
		Enabled:   row.Enabled != 0,
		Settings:  json.RawMessage(row.Settings),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}
