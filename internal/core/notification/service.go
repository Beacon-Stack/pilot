// Package notification manages notification channel configurations and
// dispatches test events to verify connectivity.
package notification

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

// ErrNotFound is returned when a notification config does not exist.
var ErrNotFound = errors.New("notification not found")

// Config is the domain representation of a stored notification configuration.
type Config struct {
	ID        string
	Name      string
	Kind      string // "webhook", "discord", "email"
	Enabled   bool
	Settings  json.RawMessage
	OnEvents  []string // event types this notification fires for
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateRequest carries the fields needed to create a notification config.
type CreateRequest struct {
	Name     string
	Kind     string
	Enabled  bool
	Settings json.RawMessage
	OnEvents []string
}

// UpdateRequest carries the fields needed to update a notification config.
type UpdateRequest = CreateRequest

// Service manages notification configurations.
type Service struct {
	q   dbsqlite.Querier
	reg *registry.Registry
}

// NewService creates a new Service.
func NewService(q dbsqlite.Querier, reg *registry.Registry) *Service {
	return &Service{q: q, reg: reg}
}

// Create persists a new notification configuration.
func (s *Service) Create(ctx context.Context, req CreateRequest) (Config, error) {
	settings := req.Settings
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}
	if _, err := s.reg.NewNotifier(req.Kind, settings); err != nil {
		return Config{}, fmt.Errorf("invalid notifier kind or settings: %w", err)
	}

	onEventsJSON, err := json.Marshal(req.OnEvents)
	if err != nil {
		return Config{}, fmt.Errorf("marshaling on_events: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateNotificationConfig(ctx, dbsqlite.CreateNotificationConfigParams{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Kind:      req.Kind,
		Enabled:   dbutil.BoolToInt(req.Enabled),
		Settings:  string(settings),
		OnEvents:  string(onEventsJSON),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return Config{}, fmt.Errorf("inserting notification config: %w", err)
	}
	return rowToConfig(row)
}

// Get returns a notification config by ID. Returns ErrNotFound if absent.
func (s *Service) Get(ctx context.Context, id string) (Config, error) {
	row, err := s.q.GetNotificationConfig(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("fetching notification %q: %w", id, err)
	}
	return rowToConfig(row)
}

// List returns all notification configs ordered by name.
func (s *Service) List(ctx context.Context) ([]Config, error) {
	rows, err := s.q.ListNotificationConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing notification configs: %w", err)
	}
	configs := make([]Config, 0, len(rows))
	for _, row := range rows {
		cfg, err := rowToConfig(row)
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// Update replaces the mutable fields of a notification config.
// Returns ErrNotFound if the config does not exist.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Config, error) {
	if _, err := s.q.GetNotificationConfig(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("fetching notification %q for update: %w", id, err)
	}

	settings := req.Settings
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}

	onEventsJSON, err := json.Marshal(req.OnEvents)
	if err != nil {
		return Config{}, fmt.Errorf("marshaling on_events: %w", err)
	}

	row, err := s.q.UpdateNotificationConfig(ctx, dbsqlite.UpdateNotificationConfigParams{
		ID:        id,
		Name:      req.Name,
		Kind:      req.Kind,
		Enabled:   dbutil.BoolToInt(req.Enabled),
		Settings:  string(settings),
		OnEvents:  string(onEventsJSON),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return Config{}, fmt.Errorf("updating notification %q: %w", id, err)
	}
	return rowToConfig(row)
}

// Delete removes a notification config. Returns ErrNotFound if absent.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.q.GetNotificationConfig(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("fetching notification %q for delete: %w", id, err)
	}
	if err := s.q.DeleteNotificationConfig(ctx, id); err != nil {
		return fmt.Errorf("deleting notification %q: %w", id, err)
	}
	return nil
}

// Test instantiates the notifier and sends a test event.
// Returns ErrNotFound if the config does not exist.
func (s *Service) Test(ctx context.Context, id string) error {
	cfg, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	n, err := s.reg.NewNotifier(cfg.Kind, cfg.Settings)
	if err != nil {
		return fmt.Errorf("instantiating notifier plugin: %w", err)
	}
	return n.Test(ctx)
}

// rowToConfig converts a DB row into the domain Config type.
func rowToConfig(row dbsqlite.NotificationConfig) (Config, error) {
	createdAt, _ := time.Parse(time.RFC3339, row.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, row.UpdatedAt)

	var onEvents []string
	if err := json.Unmarshal([]byte(row.OnEvents), &onEvents); err != nil {
		onEvents = []string{}
	}

	return Config{
		ID:        row.ID,
		Name:      row.Name,
		Kind:      row.Kind,
		Enabled:   row.Enabled != 0,
		Settings:  json.RawMessage(row.Settings),
		OnEvents:  onEvents,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
