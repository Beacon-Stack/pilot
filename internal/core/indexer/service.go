// Package indexer manages indexer configurations and orchestrates release searches.
package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	dbsqlite "github.com/screenarr/screenarr/internal/db/generated/sqlite"
	"github.com/screenarr/screenarr/internal/dbutil"
	"github.com/screenarr/screenarr/internal/events"
	"github.com/screenarr/screenarr/internal/ratelimit"
	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/pkg/plugin"
)

// ErrNotFound is returned when an indexer config does not exist.
var ErrNotFound = errors.New("indexer not found")

// Config is the domain representation of a stored indexer configuration.
type Config struct {
	ID        string
	Name      string
	Kind      string // "torznab", "newznab"
	Enabled   bool
	Priority  int
	Settings  json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateRequest carries the fields needed to create an indexer config.
type CreateRequest struct {
	Name     string
	Kind     string
	Enabled  bool
	Priority int
	Settings json.RawMessage
}

// UpdateRequest carries the fields needed to update an indexer config.
type UpdateRequest = CreateRequest

// SearchResult pairs a plugin release with its source indexer identity.
type SearchResult struct {
	plugin.Release
	// IndexerID is the DB UUID of the indexer that returned this release.
	IndexerID    string
	IndexerName  string
	QualityScore int
}

// GrabRequest carries the fields needed to record a grab history entry.
type GrabRequest struct {
	SeriesID     string
	EpisodeID    string // optional; empty for season-pack grabs
	SeasonNumber int    // optional; 0 when not known
	Release      plugin.Release
	IndexerID    string
}

// Service manages indexer configuration and search orchestration.
type Service struct {
	q     dbsqlite.Querier
	reg   *registry.Registry
	bus   *events.Bus
	rl    *ratelimit.Registry
	cache sync.Map // config ID → plugin.Indexer
}

// NewService creates a new Service.
func NewService(q dbsqlite.Querier, reg *registry.Registry, bus *events.Bus, rl *ratelimit.Registry) *Service {
	return &Service{q: q, reg: reg, bus: bus, rl: rl}
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

	now := time.Now().UTC()
	row, err := s.q.CreateIndexerConfig(ctx, dbsqlite.CreateIndexerConfigParams{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Kind:      req.Kind,
		Enabled:   dbutil.BoolToInt(req.Enabled),
		Priority:  int64(priority),
		Settings:  string(settings),
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
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

	row, err := s.q.UpdateIndexerConfig(ctx, dbsqlite.UpdateIndexerConfigParams{
		ID:        id,
		Name:      req.Name,
		Kind:      req.Kind,
		Enabled:   dbutil.BoolToInt(req.Enabled),
		Priority:  int64(priority),
		Settings:  string(settings),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
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

// Search queries all enabled indexers with the given query string and returns
// results sorted by quality score descending, then seeds descending.
// Errors from individual indexers are collected; a combined error is returned
// alongside any results that were gathered.
func (s *Service) Search(ctx context.Context, query string) ([]SearchResult, error) {
	rows, err := s.q.ListEnabledIndexers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing enabled indexers: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	type indexerResult struct {
		indexerID   string
		indexerName string
		releases    []plugin.Release
		err         error
	}

	resultsCh := make(chan indexerResult, len(rows))
	var wg sync.WaitGroup

	for _, row := range rows {
		wg.Add(1)
		go func(row dbsqlite.IndexerConfig) {
			defer wg.Done()
			cfg, _ := rowToConfig(row)
			if err := s.rl.Wait(ctx, cfg.ID, extractRateLimit(cfg.Settings)); err != nil {
				resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: err}
				return
			}
			idx, err := s.cachedIndexer(cfg.Kind, cfg.ID, cfg.Settings)
			if err != nil {
				resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: err}
				return
			}
			releases, err := idx.Search(ctx, plugin.SearchQuery{Query: query})
			resultsCh <- indexerResult{
				indexerID:   cfg.ID,
				indexerName: cfg.Name,
				releases:    releases,
				err:         err,
			}
		}(row)
	}

	wg.Wait()
	close(resultsCh)

	var allResults []SearchResult
	var errs []error

	for res := range resultsCh {
		if res.err != nil {
			errs = append(errs, fmt.Errorf("indexer %q: %w", res.indexerName, res.err))
			continue
		}
		for _, r := range res.releases {
			if r.Indexer == "" {
				r.Indexer = res.indexerName
			}
			allResults = append(allResults, SearchResult{
				Release:      r,
				IndexerID:    res.indexerID,
				IndexerName:  res.indexerName,
				QualityScore: r.Quality.Score(),
			})
		}
	}

	// Sort by quality score descending, then by seeds descending.
	sort.Slice(allResults, func(i, j int) bool {
		si, sj := allResults[i].QualityScore, allResults[j].QualityScore
		if si != sj {
			return si > sj
		}
		return allResults[i].Seeds > allResults[j].Seeds
	})

	var combinedErr error
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		combinedErr = fmt.Errorf("%d indexer(s) failed: %v", len(errs), msgs)
	}

	return allResults, combinedErr
}

// GetRecent fetches the most recent releases from all enabled indexers via
// their RSS feeds. Results from all indexers are merged and returned together.
// Errors from individual indexers are collected; a combined error is returned
// alongside any results that were gathered.
func (s *Service) GetRecent(ctx context.Context) ([]SearchResult, error) {
	rows, err := s.q.ListEnabledIndexers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing enabled indexers: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	type indexerResult struct {
		indexerID   string
		indexerName string
		releases    []plugin.Release
		err         error
	}

	resultsCh := make(chan indexerResult, len(rows))
	var wg sync.WaitGroup

	for _, row := range rows {
		wg.Add(1)
		go func(row dbsqlite.IndexerConfig) {
			defer wg.Done()
			cfg, _ := rowToConfig(row)
			if err := s.rl.Wait(ctx, cfg.ID, extractRateLimit(cfg.Settings)); err != nil {
				resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: err}
				return
			}
			idx, err := s.cachedIndexer(cfg.Kind, cfg.ID, cfg.Settings)
			if err != nil {
				resultsCh <- indexerResult{indexerID: cfg.ID, indexerName: cfg.Name, err: err}
				return
			}
			releases, err := idx.GetRecent(ctx)
			resultsCh <- indexerResult{
				indexerID:   cfg.ID,
				indexerName: cfg.Name,
				releases:    releases,
				err:         err,
			}
		}(row)
	}

	wg.Wait()
	close(resultsCh)

	var allResults []SearchResult
	var errs []error

	for res := range resultsCh {
		if res.err != nil {
			errs = append(errs, fmt.Errorf("indexer %q: %w", res.indexerName, res.err))
			continue
		}
		for _, r := range res.releases {
			if r.Indexer == "" {
				r.Indexer = res.indexerName
			}
			allResults = append(allResults, SearchResult{
				Release:      r,
				IndexerID:    res.indexerID,
				IndexerName:  res.indexerName,
				QualityScore: r.Quality.Score(),
			})
		}
	}

	var combinedErr error
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		combinedErr = fmt.Errorf("%d indexer(s) failed: %v", len(errs), msgs)
	}

	return allResults, combinedErr
}

// GrabHistory returns the grab history for a series, newest first.
func (s *Service) GrabHistory(ctx context.Context, seriesID string) ([]dbsqlite.GrabHistory, error) {
	return s.q.ListGrabHistoryBySeries(ctx, seriesID)
}

// CreateGrab records a grab history entry linking the release to the series
// and optionally to a specific episode. Pass empty strings for fields that are
// not applicable (e.g. season-pack grabs have no episodeID).
func (s *Service) CreateGrab(ctx context.Context, req GrabRequest) (dbsqlite.GrabHistory, error) {
	var idxID *string
	if req.IndexerID != "" {
		idxID = &req.IndexerID
	}

	var episodeID *string
	if req.EpisodeID != "" {
		episodeID = &req.EpisodeID
	}

	var seasonNumber *int64
	if req.SeasonNumber > 0 {
		sn := int64(req.SeasonNumber)
		seasonNumber = &sn
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateGrabHistory(ctx, dbsqlite.CreateGrabHistoryParams{
		ID:                uuid.New().String(),
		SeriesID:          req.SeriesID,
		EpisodeID:         episodeID,
		SeasonNumber:      seasonNumber,
		IndexerID:         idxID,
		ReleaseGuid:       req.Release.GUID,
		ReleaseTitle:      req.Release.Title,
		ReleaseSource:     string(req.Release.Quality.Source),
		ReleaseResolution: string(req.Release.Quality.Resolution),
		ReleaseCodec:      string(req.Release.Quality.Codec),
		ReleaseHdr:        string(req.Release.Quality.HDR),
		Protocol:          string(req.Release.Protocol),
		Size:              req.Release.Size,
		DownloadClientID:  nil,
		ClientItemID:      nil,
		DownloadStatus:    "queued",
		ScoreBreakdown:    nil,
		GrabbedAt:         now,
	})
	if err != nil {
		return dbsqlite.GrabHistory{}, fmt.Errorf("recording grab history: %w", err)
	}

	if s.bus != nil {
		s.bus.Publish(ctx, events.Event{
			Type:   events.TypeEpisodeGrabbed,
			ShowID: req.SeriesID,
			Data:   map[string]any{"title": req.Release.Title},
		})
	}

	return row, nil
}

// UpdateGrabDownloadClient records the download client and client-assigned item ID
// on an existing grab history entry. Called after a release is successfully submitted
// to a download client.
func (s *Service) UpdateGrabDownloadClient(ctx context.Context, arg dbsqlite.UpdateGrabDownloadClientParams) error {
	if err := s.q.UpdateGrabDownloadClient(ctx, arg); err != nil {
		return fmt.Errorf("updating grab download client: %w", err)
	}
	return nil
}

func rowToConfig(row dbsqlite.IndexerConfig) (Config, error) {
	createdAt, err := time.Parse(time.RFC3339, row.CreatedAt)
	if err != nil {
		createdAt = time.Time{}
	}
	updatedAt, err := time.Parse(time.RFC3339, row.UpdatedAt)
	if err != nil {
		updatedAt = time.Time{}
	}
	return Config{
		ID:        row.ID,
		Name:      row.Name,
		Kind:      row.Kind,
		Enabled:   row.Enabled != 0,
		Priority:  int(row.Priority),
		Settings:  json.RawMessage(row.Settings),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
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
