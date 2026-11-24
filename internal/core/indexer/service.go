// Package indexer manages indexer configurations and runs release
// searches: it queries every enabled indexer, tags releases that fail a
// filter, and ranks the results (see searchResultLess).
package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/dbutil"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/internal/ratelimit"
	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ErrNotFound is returned when an indexer config does not exist.
var ErrNotFound = errors.New("indexer not found")

// Config is the domain representation of a stored indexer configuration.
type Config struct {
	ID         string
	Name       string
	Kind       string // "torznab", "newznab"
	Enabled    bool
	Priority   int
	MinSeeders int // releases below this threshold get tagged with a filter reason
	Settings   json.RawMessage
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateRequest carries the fields needed to create an indexer config.
type CreateRequest struct {
	Name       string
	Kind       string
	Enabled    bool
	Priority   int
	MinSeeders int // 0 means "use the sensible default (5)"
	Settings   json.RawMessage
}

// UpdateRequest carries the fields needed to update an indexer config.
type UpdateRequest = CreateRequest

type Service struct {
	q      db.Querier
	reg    *registry.Registry
	bus    *events.Bus
	rl     *ratelimit.Registry
	logger *slog.Logger
	cache  sync.Map      // config ID → plugin.Indexer
	sem    chan struct{} // bounds concurrent indexer searches
	health indexerHealth // recent outcomes per indexer; see struggleTimeout
	// healthSaveMu serializes saveHealth.
	healthSaveMu sync.Mutex
}

// NewService creates a new Service. A nil logger falls back to slog.Default()
// so existing callers continue to work without migration.
func NewService(q db.Querier, reg *registry.Registry, bus *events.Bus, rl *ratelimit.Registry, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		q:      q,
		reg:    reg,
		bus:    bus,
		rl:     rl,
		logger: logger,
		sem:    make(chan struct{}, maxConcurrentIndexerSearches),
	}
}

// cachedIndexer returns a cached or freshly-created indexer for the given config.
func (s *Service) cachedIndexer(kind, id string, settings json.RawMessage) (plugin.Indexer, error) {
	if v, ok := s.cache.Load(id); ok {
		return v.(plugin.Indexer), nil
	}
	idx, err := s.reg.NewIndexer(kind, settings)
	if err != nil {
		return nil, err
	}
	actual, _ := s.cache.LoadOrStore(id, idx)
	return actual.(plugin.Indexer), nil
}

// evictIndexer removes a cached indexer instance.
func (s *Service) evictIndexer(id string) {
	s.cache.Delete(id)
}

// Create persists a new indexer configuration.
func (s *Service) Create(ctx context.Context, req CreateRequest) (Config, error) {
	settings := req.Settings
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}
	// Validate that the kind is registered.
	if _, err := s.reg.NewIndexer(req.Kind, settings); err != nil {
		return Config{}, fmt.Errorf("invalid indexer kind or settings: %w", err)
	}

	priority := req.Priority
	if priority <= 0 {
		priority = 25
	}

	minSeeders := req.MinSeeders
	if minSeeders <= 0 {
		minSeeders = 5 // sensible default for public trackers
	}

	now := time.Now().UTC()
	row, err := s.q.CreateIndexerConfig(ctx, db.CreateIndexerConfigParams{
		ID:         uuid.New().String(),
		Name:       req.Name,
		Kind:       req.Kind,
		Enabled:    req.Enabled,
		Priority:   int64(priority),
		Settings:   string(settings),
		MinSeeders: int64(minSeeders),
		CreatedAt:  now.Format(time.RFC3339),
		UpdatedAt:  now.Format(time.RFC3339),
	})
	if err != nil {
		return Config{}, fmt.Errorf("inserting indexer config: %w", err)
	}

	return rowToConfig(row)
}

// Get returns an indexer config by ID. Returns ErrNotFound if absent.
func (s *Service) Get(ctx context.Context, id string) (Config, error) {
	row, err := s.q.GetIndexerConfig(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("fetching indexer %q: %w", id, err)
	}
	return rowToConfig(row)
}

// List returns all indexer configs, ordered by priority then name.
func (s *Service) List(ctx context.Context) ([]Config, error) {
	rows, err := s.q.ListIndexerConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing indexer configs: %w", err)
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

// Update replaces the mutable fields of an indexer config.
// Returns ErrNotFound if the indexer does not exist.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Config, error) {
	existing, err := s.q.GetIndexerConfig(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("fetching indexer %q for update: %w", id, err)
	}

	// Merge: keys absent from req.Settings are preserved from existing settings.
	// This ensures secret fields (API keys) are not erased when omitted by the client.
	settings := dbutil.MergeSettings(json.RawMessage(existing.Settings), req.Settings)
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}

	priority := req.Priority
	if priority <= 0 {
		priority = 25
	}

	minSeeders := req.MinSeeders
	if minSeeders <= 0 {
		// Preserve existing value if the request didn't set one.
		minSeeders = int(existing.MinSeeders)
		if minSeeders <= 0 {
			minSeeders = 5
		}
	}

	row, err := s.q.UpdateIndexerConfig(ctx, db.UpdateIndexerConfigParams{
		ID:         id,
		Name:       req.Name,
		Kind:       req.Kind,
		Enabled:    req.Enabled,
		Priority:   int64(priority),
		Settings:   string(settings),
		MinSeeders: int64(minSeeders),
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return Config{}, fmt.Errorf("updating indexer %q: %w", id, err)
	}
	s.evictIndexer(id)
	return rowToConfig(row)
}

// Delete removes an indexer config. Returns ErrNotFound if absent.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.q.GetIndexerConfig(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("fetching indexer %q for delete: %w", id, err)
	}
	if err := s.q.DeleteIndexerConfig(ctx, id); err != nil {
		return fmt.Errorf("deleting indexer %q: %w", id, err)
	}
	s.rl.Remove(id)
	s.evictIndexer(id)
	return nil
}

// Test instantiates the indexer plugin and verifies connectivity.
func (s *Service) Test(ctx context.Context, id string) error {
	cfg, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.rl.Wait(ctx, cfg.ID, extractRateLimit(cfg.Settings)); err != nil {
		return fmt.Errorf("rate limit wait: %w", err)
	}
	idx, err := s.reg.NewIndexer(cfg.Kind, cfg.Settings)
	if err != nil {
		return fmt.Errorf("instantiating indexer plugin: %w", err)
	}
	return idx.Test(ctx)
}

func rowToConfig(row db.IndexerConfig) (Config, error) {
	createdAt, err := time.Parse(time.RFC3339, row.CreatedAt)
	if err != nil {
		createdAt = time.Time{}
	}
	updatedAt, err := time.Parse(time.RFC3339, row.UpdatedAt)
	if err != nil {
		updatedAt = time.Time{}
	}
	return Config{
		ID:         row.ID,
		Name:       row.Name,
		Kind:       row.Kind,
		Enabled:    row.Enabled,
		Priority:   int(row.Priority),
		MinSeeders: int(row.MinSeeders),
		Settings:   json.RawMessage(row.Settings),
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

// extractRateLimit reads the rate_limit field from an indexer's settings JSON.
// Returns 0 (unlimited) if the field is absent or unparseable.
func extractRateLimit(settings json.RawMessage) int {
	var s struct {
		RateLimit int `json:"rate_limit"`
	}
	_ = json.Unmarshal(settings, &s)
	return s.RateLimit
}
