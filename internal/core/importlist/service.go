// Package importlist manages import list configurations and syncs series from
// external sources (TMDb, Trakt, Plex, etc.) into the library.
package importlist

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
	"github.com/beacon-stack/pilot/internal/core/show"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/internal/trakt"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ErrNotFound is returned when an import list config does not exist.
var ErrNotFound = errors.New("import list not found")

// maxSearchOnAdd caps the number of series that will be auto-searched after
// a sync to avoid overwhelming indexers.
const maxSearchOnAdd = 20

// Config is the domain representation of an import list configuration.
type Config struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Kind             string          `json:"kind"`
	Enabled          bool            `json:"enabled"`
	Settings         json.RawMessage `json:"settings"`
	SearchOnAdd      bool            `json:"search_on_add"`
	Monitor          bool            `json:"monitor"`
	MonitorType      string          `json:"monitor_type"`
	QualityProfileID string          `json:"quality_profile_id"`
	LibraryID        string          `json:"library_id"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// CreateRequest holds the fields for creating an import list config.
type CreateRequest struct {
	Name             string          `json:"name"`
	Kind             string          `json:"kind"`
	Enabled          bool            `json:"enabled"`
	Settings         json.RawMessage `json:"settings"`
	SearchOnAdd      bool            `json:"search_on_add"`
	Monitor          bool            `json:"monitor"`
	MonitorType      string          `json:"monitor_type"`
	QualityProfileID string          `json:"quality_profile_id"`
	LibraryID        string          `json:"library_id"`
}

// UpdateRequest holds the fields for updating an import list config.
type UpdateRequest = CreateRequest

// Exclusion is a series excluded from import list syncs.
type Exclusion struct {
	ID        string    `json:"id"`
	TMDbID    int       `json:"tmdb_id"`
	Title     string    `json:"title"`
	Year      int       `json:"year"`
	CreatedAt time.Time `json:"created_at"`
}

// SyncResult summarises a completed import list sync.
type SyncResult struct {
	ListsProcessed int      `json:"lists_processed"`
	SeriesAdded    int      `json:"series_added"`
	SeriesSkipped  int      `json:"series_skipped"`
	Errors         []string `json:"errors"`
}

// Service manages import list configurations and executes syncs.
type Service struct {
	q           db.Querier
	reg         *registry.Registry
	showSvc     *show.Service
	tmdbClient  *tmdbtv.Client
	traktClient *trakt.Client
	logger      *slog.Logger
	syncMu      sync.Mutex
}

// NewService creates a new import list Service.
// tmdbClient and traktClient may be nil.
func NewService(
	q db.Querier,
	reg *registry.Registry,
	showSvc *show.Service,
	tmdbClient *tmdbtv.Client,
	traktClient *trakt.Client,
	logger *slog.Logger,
) *Service {
	return &Service{
		q:           q,
		reg:         reg,
		showSvc:     showSvc,
		tmdbClient:  tmdbClient,
		traktClient: traktClient,
		logger:      logger,
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// Create adds a new import list configuration.
func (s *Service) Create(ctx context.Context, req CreateRequest) (Config, error) {
	if _, err := s.reg.NewImportList(req.Kind, req.Settings); err != nil {
		return Config{}, fmt.Errorf("validating import list settings: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateImportListConfig(ctx, db.CreateImportListConfigParams{
		ID:               uuid.New().String(),
		Name:             req.Name,
		Kind:             req.Kind,
		Enabled:          req.Enabled,
		Settings:         string(req.Settings),
		SearchOnAdd:      req.SearchOnAdd,
		Monitor:          req.Monitor,
		MonitorType:      req.MonitorType,
		QualityProfileID: req.QualityProfileID,
		LibraryID:        req.LibraryID,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return Config{}, fmt.Errorf("creating import list config: %w", err)
	}
	return rowToConfig(row), nil
}

// Get returns an import list config by ID.
func (s *Service) Get(ctx context.Context, id string) (Config, error) {
	row, err := s.q.GetImportListConfig(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("getting import list config %q: %w", id, err)
	}
	return rowToConfig(row), nil
}

// List returns all import list configs, ordered by name.
func (s *Service) List(ctx context.Context) ([]Config, error) {
	rows, err := s.q.ListImportListConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing import list configs: %w", err)
	}
	cfgs := make([]Config, len(rows))
	for i, r := range rows {
		cfgs[i] = rowToConfig(r)
	}
	return cfgs, nil
}

// Update modifies an existing import list config.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Config, error) {
	existing, err := s.q.GetImportListConfig(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Config{}, ErrNotFound
		}
		return Config{}, fmt.Errorf("fetching import list config %q for update: %w", id, err)
	}

	settings := dbutil.MergeSettings(json.RawMessage(existing.Settings), req.Settings)

	if _, err := s.reg.NewImportList(req.Kind, settings); err != nil {
		return Config{}, fmt.Errorf("validating import list settings: %w", err)
	}

	row, err := s.q.UpdateImportListConfig(ctx, db.UpdateImportListConfigParams{
		Name:             req.Name,
		Kind:             req.Kind,
		Enabled:          req.Enabled,
		Settings:         string(settings),
		SearchOnAdd:      req.SearchOnAdd,
		Monitor:          req.Monitor,
		MonitorType:      req.MonitorType,
		QualityProfileID: req.QualityProfileID,
		LibraryID:        req.LibraryID,
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
		ID:               id,
	})
	if err != nil {
		return Config{}, fmt.Errorf("updating import list config %q: %w", id, err)
	}
	return rowToConfig(row), nil
}

// Delete removes an import list config by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.q.DeleteImportListConfig(ctx, id); err != nil {
		return fmt.Errorf("deleting import list config %q: %w", id, err)
	}
	return nil
}

// Test validates that the import list plugin can connect to its source.
func (s *Service) Test(ctx context.Context, id string) error {
	cfg, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	pl, err := s.reg.NewImportList(cfg.Kind, cfg.Settings)
	if err != nil {
		return fmt.Errorf("instantiating import list plugin: %w", err)
	}
	s.injectClients(pl)
	return pl.Test(ctx)
}

// ---------------------------------------------------------------------------
// Preview
// ---------------------------------------------------------------------------

// PreviewItem is a single series from a list preview, including poster info.
type PreviewItem struct {
	TMDbID     int    `json:"tmdb_id"`
	Title      string `json:"title"`
	Year       int    `json:"year"`
	PosterPath string `json:"poster_path,omitempty"`
}

// Preview instantiates a plugin from the given kind + settings, fetches its
// items, and returns them as preview items (without adding anything to the library).
func (s *Service) Preview(ctx context.Context, kind string, settings json.RawMessage) ([]PreviewItem, error) {
	pl, err := s.reg.NewImportList(kind, settings)
	if err != nil {
		return nil, fmt.Errorf("instantiating plugin: %w", err)
	}
	s.injectClients(pl)

	items, err := pl.Fetch(ctx)
	if err != nil {
		return nil, err
	}

	previews := make([]PreviewItem, 0, len(items))
	for _, item := range items {
		if item.TMDbID == 0 {
			continue
		}
		previews = append(previews, PreviewItem{
			TMDbID:     item.TMDbID,
			Title:      item.Title,
			Year:       item.Year,
			PosterPath: item.PosterPath,
		})
	}
	return previews, nil
}

// ---------------------------------------------------------------------------
// Exclusions
// ---------------------------------------------------------------------------

// CreateExclusion adds a series to the import exclusion list.
func (s *Service) CreateExclusion(ctx context.Context, tmdbID int, title string, year int) (Exclusion, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateImportExclusion(ctx, db.CreateImportExclusionParams{
		ID:        uuid.New().String(),
		TmdbID:    int32(tmdbID),
		Title:     title,
		Year:      int32(year),
		CreatedAt: now,
	})
	if err != nil {
		if dbutil.IsUniqueViolation(err) {
			return Exclusion{}, fmt.Errorf("series (tmdb:%d) is already excluded", tmdbID)
		}
		return Exclusion{}, fmt.Errorf("creating import exclusion: %w", err)
	}
	return rowToExclusion(row), nil
}

// ListExclusions returns all import exclusions.
func (s *Service) ListExclusions(ctx context.Context) ([]Exclusion, error) {
	rows, err := s.q.ListImportExclusions(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing import exclusions: %w", err)
	}
	excl := make([]Exclusion, len(rows))
	for i, r := range rows {
		excl[i] = rowToExclusion(r)
	}
	return excl, nil
}

// DeleteExclusion removes an import exclusion by ID.
func (s *Service) DeleteExclusion(ctx context.Context, id string) error {
	if err := s.q.DeleteImportExclusion(ctx, id); err != nil {
		return fmt.Errorf("deleting import exclusion %q: %w", id, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Sync
// ---------------------------------------------------------------------------

// Sync fetches all enabled import lists and adds new series to the library.
// Returns immediately if a sync is already running.
func (s *Service) Sync(ctx context.Context) SyncResult {
	if !s.syncMu.TryLock() {
		return SyncResult{Errors: []string{"sync already running"}}
	}
	defer s.syncMu.Unlock()

	return s.doSync(ctx, nil)
}

// SyncOne syncs a single import list by ID.
func (s *Service) SyncOne(ctx context.Context, id string) SyncResult {
	if !s.syncMu.TryLock() {
		return SyncResult{Errors: []string{"sync already running"}}
	}
	defer s.syncMu.Unlock()

	return s.doSync(ctx, &id)
}

func (s *Service) doSync(ctx context.Context, onlyID *string) SyncResult {
	result := SyncResult{Errors: []string{}}

	// Load enabled lists (or a specific one).
	var lists []db.ImportListConfig
	var err error
	if onlyID != nil {
		row, getErr := s.q.GetImportListConfig(ctx, *onlyID)
		if getErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("list %q: %v", *onlyID, getErr))
			return result
		}
		lists = []db.ImportListConfig{row}
	} else {
		lists, err = s.q.ListEnabledImportLists(ctx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("loading lists: %v", err))
			return result
		}
	}

	// Build set of existing TMDb IDs in library.
	existingIDs, err := s.q.ListAllTMDBIDs(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("loading existing tmdb ids: %v", err))
		return result
	}
	existingSet := make(map[int32]bool, len(existingIDs))
	for _, id := range existingIDs {
		existingSet[id] = true
	}

	// Build exclusion set.
	excludedIDs, err := s.q.ListExcludedTMDBIDs(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("loading exclusions: %v", err))
		return result
	}
	excludedSet := make(map[int32]bool, len(excludedIDs))
	for _, id := range excludedIDs {
		excludedSet[id] = true
	}

	searchCount := 0

	// Process lists sequentially to avoid rate limit issues.
	for _, listRow := range lists {
		result.ListsProcessed++
		cfg := rowToConfig(listRow)

		pl, plErr := s.reg.NewImportList(cfg.Kind, cfg.Settings)
		if plErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("list %q: %v", cfg.Name, plErr))
			continue
		}
		s.injectClients(pl)

		items, fetchErr := pl.Fetch(ctx)
		if fetchErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("list %q fetch: %v", cfg.Name, fetchErr))
			continue
		}

		s.logger.Info("import list fetched",
			slog.String("list", cfg.Name),
			slog.String("kind", cfg.Kind),
			slog.Int("items", len(items)),
		)

		for _, item := range items {
			if item.TMDbID == 0 {
				continue
			}
			tmdbID := int32(item.TMDbID)
			if existingSet[tmdbID] {
				result.SeriesSkipped++
				continue
			}
			if excludedSet[tmdbID] {
				result.SeriesSkipped++
				continue
			}

			_, addErr := s.showSvc.Add(ctx, show.AddRequest{
				TMDBID:           item.TMDbID,
				LibraryID:        cfg.LibraryID,
				QualityProfileID: cfg.QualityProfileID,
				Monitored:        cfg.Monitor,
				MonitorType:      cfg.MonitorType,
				SeriesType:       "standard",
			})
			if addErr != nil {
				if errors.Is(addErr, show.ErrAlreadyExists) {
					existingSet[tmdbID] = true
					result.SeriesSkipped++
					continue
				}
				result.Errors = append(result.Errors, fmt.Sprintf("add %q (tmdb:%d): %v", item.Title, item.TMDbID, addErr))
				continue
			}

			result.SeriesAdded++
			existingSet[tmdbID] = true

			if cfg.SearchOnAdd && searchCount < maxSearchOnAdd {
				searchCount++
				// Search-on-add is not yet implemented; will be wired when autosearch is available.
			}
		}
	}

	return result
}

// injectClients calls Set*Client on plugins that implement injectable interfaces.
func (s *Service) injectClients(pl plugin.ImportList) {
	if injectable, ok := pl.(plugin.TMDBInjectable); ok && s.tmdbClient != nil {
		injectable.SetTMDBClient(s.tmdbClient)
	}
	if injectable, ok := pl.(plugin.TraktInjectable); ok && s.traktClient != nil {
		injectable.SetTraktClient(s.traktClient)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func rowToConfig(r db.ImportListConfig) Config {
	ca, _ := time.Parse(time.RFC3339, r.CreatedAt)
	ua, _ := time.Parse(time.RFC3339, r.UpdatedAt)
	return Config{
		ID:               r.ID,
		Name:             r.Name,
		Kind:             r.Kind,
		Enabled:          r.Enabled,
		Settings:         json.RawMessage(r.Settings),
		SearchOnAdd:      r.SearchOnAdd,
		Monitor:          r.Monitor,
		MonitorType:      r.MonitorType,
		QualityProfileID: r.QualityProfileID,
		LibraryID:        r.LibraryID,
		CreatedAt:        ca,
		UpdatedAt:        ua,
	}
}

func rowToExclusion(r db.ImportExclusion) Exclusion {
	ca, _ := time.Parse(time.RFC3339, r.CreatedAt)
	return Exclusion{
		ID:        r.ID,
		TMDbID:    int(r.TmdbID),
		Title:     r.Title,
		Year:      int(r.Year),
		CreatedAt: ca,
	}
}
