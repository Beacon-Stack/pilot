// Package show manages TV series records in the Pilot library.
package show

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	dbsqlite "github.com/beacon-media/pilot/internal/db/generated/sqlite"
	"github.com/beacon-media/pilot/internal/events"
	"github.com/beacon-media/pilot/internal/metadata/tmdbtv"
)

// Sentinel errors returned by Service methods.
var (
	ErrNotFound              = errors.New("series not found")
	ErrAlreadyExists         = errors.New("series already in library")
	ErrMetadataNotConfigured = errors.New("metadata provider not configured")
	ErrFileNotFound          = errors.New("episode file not found")
)

// RenamePreview describes a single file rename operation (or proposed rename
// when dry_run=true).
type RenamePreview struct {
	FileID  string
	OldPath string
	NewPath string
}

// MetadataProvider fetches TV series metadata from an external source.
type MetadataProvider interface {
	SearchSeries(ctx context.Context, query string, year int) ([]tmdbtv.SearchResult, error)
	GetSeries(ctx context.Context, tmdbID int) (*tmdbtv.SeriesDetail, error)
	GetSeasonEpisodes(ctx context.Context, tmdbID int, seasonNum int) ([]tmdbtv.EpisodeDetail, error)
}

// Series is the domain representation of a TV series record.
type Series struct {
	ID                  string
	TMDBID              int
	IMDBID              string
	Title               string
	SortTitle           string
	Year                int
	Overview            string
	RuntimeMinutes      int
	Genres              []string
	PosterURL           string
	FanartURL           string
	Status              string
	SeriesType          string
	MonitorType         string
	Network             string
	AirTime             string
	Certification       string
	Monitored           bool
	LibraryID           string
	QualityProfileID    string
	Path                string
	AddedAt             time.Time
	UpdatedAt           time.Time
	MetadataRefreshedAt *time.Time
	EpisodeCount        int64
	EpisodeFileCount    int64
}

// Season is the domain representation of a season record.
type Season struct {
	ID           string
	SeriesID     string
	SeasonNumber int
	Monitored    bool
}

// Episode is the domain representation of an episode record.
type Episode struct {
	ID             string
	SeriesID       string
	SeasonID       string
	SeasonNumber   int
	EpisodeNumber  int
	AbsoluteNumber *int
	AirDate        string
	Title          string
	Overview       string
	Monitored      bool
	HasFile        bool
}

// AddRequest carries the fields needed to add a series to the library.
type AddRequest struct {
	TMDBID           int
	LibraryID        string
	QualityProfileID string
	Monitored        bool
	MonitorType      string // "all", "future", "missing", "none", "pilot", "first_season", "last_season", "existing"
	SeriesType       string // "standard", "anime", "daily"
}

// UpdateRequest carries the mutable fields for updating a series.
type UpdateRequest struct {
	Title            string
	Monitored        bool
	LibraryID        string
	QualityProfileID string
	SeriesType       string
	Path             string
}

// ListRequest carries filter and pagination options for listing series.
type ListRequest struct {
	LibraryID string // empty = all libraries
	Page      int    // 1-indexed; defaults to 1
	PerPage   int    // defaults to 50
}

// ListResult is the paginated response from List.
type ListResult struct {
	Series  []Series
	Total   int64
	Page    int
	PerPage int
}

// LookupRequest carries parameters for searching the metadata provider
// without adding a series to the library.
type LookupRequest struct {
	Query  string
	TMDBID int // if set, fetch exact series; Query is ignored
	Year   int // optional year filter for query search
}

// seasonEntry groups a freshly created season row with its episode rows.
// Used only during the Add flow to carry data into applyMonitorType.
type seasonEntry struct {
	row      dbsqlite.Season
	episodes []dbsqlite.Episode
}

// Service manages TV series, seasons, and episodes.
type Service struct {
	q      dbsqlite.Querier
	meta   MetadataProvider
	bus    *events.Bus
	logger *slog.Logger
}

// NewService creates a new Service. meta may be nil; methods that require it
// will return ErrMetadataNotConfigured.
func NewService(q dbsqlite.Querier, meta MetadataProvider, bus *events.Bus, logger *slog.Logger) *Service {
	return &Service{
		q:      q,
		meta:   meta,
		bus:    bus,
		logger: logger,
	}
}

// Add fetches metadata for the given TMDB ID, creates the series row, creates
// all season and episode rows, applies monitor_type logic, and publishes a
// TypeShowAdded event.
func (s *Service) Add(ctx context.Context, req AddRequest) (Series, error) {
	if s.meta == nil {
		return Series{}, ErrMetadataNotConfigured
	}

	// Reject duplicates before hitting the metadata API.
	_, err := s.q.GetSeriesByTMDBID(ctx, int64(req.TMDBID))
	if err == nil {
		return Series{}, ErrAlreadyExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Series{}, fmt.Errorf("check duplicate: %w", err)
	}

	detail, err := s.meta.GetSeries(ctx, req.TMDBID)
	if err != nil {
		return Series{}, fmt.Errorf("fetch series metadata: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	nowTime := time.Now().UTC()
	metaNow := nowTime.Format(time.RFC3339)

	genresJSON, err := json.Marshal(detail.Genres)
	if err != nil {
		return Series{}, fmt.Errorf("marshal genres: %w", err)
	}

	seriesID := uuid.NewString()
	monitoredInt := boolToInt(req.Monitored)

	monitorType := req.MonitorType
	if monitorType == "" {
		monitorType = "all"
	}
	seriesType := req.SeriesType
	if seriesType == "" {
		seriesType = "standard"
	}

	var runtimePtr *int64
	if detail.RuntimeMinutes > 0 {
		rt := int64(detail.RuntimeMinutes)
		runtimePtr = &rt
	}

	posterURL := nullableString(detail.PosterPath)
	fanartURL := nullableString(detail.BackdropPath)
	network := nullableString(detail.Network)

	row, err := s.q.CreateSeries(ctx, dbsqlite.CreateSeriesParams{
		ID:                  seriesID,
		TmdbID:              int64(detail.ID),
		ImdbID:              nil,
		Title:               detail.Title,
		SortTitle:           sortTitle(detail.Title),
		Year:                int64(detail.Year),
		Overview:            detail.Overview,
		RuntimeMinutes:      runtimePtr,
		GenresJson:          string(genresJSON),
		PosterUrl:           posterURL,
		FanartUrl:           fanartURL,
		Status:              detail.Status,
		SeriesType:          seriesType,
		MonitorType:         monitorType,
		Network:             network,
		AirTime:             nil,
		Certification:       nil,
		Monitored:           monitoredInt,
		LibraryID:           req.LibraryID,
		QualityProfileID:    req.QualityProfileID,
		Path:                nil,
		AddedAt:             now,
		UpdatedAt:           now,
		MetadataRefreshedAt: &metaNow,
	})
	if err != nil {
		return Series{}, fmt.Errorf("create series: %w", err)
	}

	// Create seasons and episodes. Season 0 (specials) is included if present.
	// We track season rows by season_number so we can apply monitor_type later.
	seasonMap := make(map[int]seasonEntry, len(detail.Seasons))

	for _, ss := range detail.Seasons {
		seasonID := uuid.NewString()
		seasonRow, err := s.q.CreateSeason(ctx, dbsqlite.CreateSeasonParams{
			ID:           seasonID,
			SeriesID:     seriesID,
			SeasonNumber: int64(ss.SeasonNumber),
			Monitored:    1, // will be overridden by monitor_type pass below
		})
		if err != nil {
			return Series{}, fmt.Errorf("create season %d: %w", ss.SeasonNumber, err)
		}

		// Fetch episodes for this season from the provider.
		epDetails, err := s.meta.GetSeasonEpisodes(ctx, req.TMDBID, ss.SeasonNumber)
		if err != nil {
			s.logger.Warn("failed to fetch season episodes",
				"series_tmdb_id", req.TMDBID,
				"season", ss.SeasonNumber,
				"error", err,
			)
			epDetails = nil
		}

		var episodeRows []dbsqlite.Episode
		for _, ep := range epDetails {
			epID := uuid.NewString()
			airDate := nullableString(ep.AirDate)
			epRow, err := s.q.CreateEpisode(ctx, dbsqlite.CreateEpisodeParams{
				ID:             epID,
				SeriesID:       seriesID,
				SeasonID:       seasonID,
				SeasonNumber:   int64(ep.SeasonNumber),
				EpisodeNumber:  int64(ep.EpisodeNumber),
				AbsoluteNumber: nil,
				AirDate:        airDate,
				Title:          ep.Title,
				Overview:       ep.Overview,
				Monitored:      1, // will be overridden by monitor_type pass below
				HasFile:        0,
			})
			if err != nil {
				return Series{}, fmt.Errorf("create episode S%02dE%02d: %w", ep.SeasonNumber, ep.EpisodeNumber, err)
			}
			episodeRows = append(episodeRows, epRow)
		}

		seasonMap[ss.SeasonNumber] = seasonEntry{row: seasonRow, episodes: episodeRows}
	}

	// Apply monitor_type logic to seasons and episodes.
	if err := s.applyMonitorType(ctx, monitorType, seasonMap); err != nil {
		return Series{}, fmt.Errorf("apply monitor type: %w", err)
	}

	result, err := s.buildSeries(ctx, row)
	if err != nil {
		return Series{}, err
	}

	if s.bus != nil {
		s.bus.Publish(ctx, events.Event{
			Type:   events.TypeShowAdded,
			ShowID: seriesID,
			Data:   map[string]any{"title": detail.Title, "tmdb_id": detail.ID},
		})
	}

	s.logger.Info("series added", "id", seriesID, "title", detail.Title, "tmdb_id", detail.ID)
	return result, nil
}

// applyMonitorType sets episode and season monitored flags according to the
// requested monitor strategy. seasonMap contains the freshly created rows.
func (s *Service) applyMonitorType(
	ctx context.Context,
	monitorType string,
	seasonMap map[int]seasonEntry,
) error {
	today := time.Now().UTC().Format("2006-01-02")

	// Determine the highest non-special season number for "last_season".
	lastSeasonNum := 0
	for num := range seasonMap {
		if num > lastSeasonNum {
			lastSeasonNum = num
		}
	}

	for seasonNum, entry := range seasonMap {
		for _, ep := range entry.episodes {
			var monitored int64

			switch monitorType {
			case "all":
				monitored = 1

			case "future":
				// Monitor episodes with no air date or air date after today.
				if ep.AirDate == nil || *ep.AirDate == "" || *ep.AirDate > today {
					monitored = 1
				}

			case "missing":
				// For a new series, no files exist — monitor all aired episodes.
				if ep.AirDate != nil && *ep.AirDate != "" && *ep.AirDate <= today {
					monitored = 1
				}

			case "none":
				monitored = 0

			case "pilot":
				if seasonNum == 1 && ep.EpisodeNumber == 1 {
					monitored = 1
				}

			case "first_season":
				if seasonNum == 1 {
					monitored = 1
				}

			case "last_season":
				if seasonNum == lastSeasonNum {
					monitored = 1
				}

			case "existing":
				// New series has no files — nothing is monitored.
				monitored = 0

			default:
				monitored = 1
			}

			if err := s.q.UpdateEpisodeMonitored(ctx, dbsqlite.UpdateEpisodeMonitoredParams{
				Monitored: monitored,
				ID:        ep.ID,
			}); err != nil {
				return fmt.Errorf("update episode monitored %s: %w", ep.ID, err)
			}
		}

		// Season is monitored if any episode within it is monitored.
		seasonMonitored := int64(0)
		switch monitorType {
		case "all":
			seasonMonitored = 1
		case "future", "missing":
			seasonMonitored = 1
		case "first_season":
			if seasonNum == 1 {
				seasonMonitored = 1
			}
		case "last_season":
			if seasonNum == lastSeasonNum {
				seasonMonitored = 1
			}
		case "pilot":
			if seasonNum == 1 {
				seasonMonitored = 1
			}
		case "none", "existing":
			seasonMonitored = 0
		default:
			seasonMonitored = 1
		}

		if err := s.q.UpdateSeasonMonitored(ctx, dbsqlite.UpdateSeasonMonitoredParams{
			Monitored: seasonMonitored,
			ID:        entry.row.ID,
		}); err != nil {
			return fmt.Errorf("update season monitored %s: %w", entry.row.ID, err)
		}
	}

	return nil
}

// Get returns a single series by ID, with episode counts populated.
func (s *Service) Get(ctx context.Context, id string) (Series, error) {
	row, err := s.q.GetSeries(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Series{}, ErrNotFound
		}
		return Series{}, fmt.Errorf("get series: %w", err)
	}
	return s.buildSeries(ctx, row)
}

// List returns a paginated list of series, optionally filtered by library.
func (s *Service) List(ctx context.Context, req ListRequest) (ListResult, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 {
		perPage = 50
	}

	offset := int64((page - 1) * perPage)
	limit := int64(perPage)

	var rows []dbsqlite.Series
	var total int64

	if req.LibraryID != "" {
		var err error
		total, err = s.q.CountSeriesByLibrary(ctx, req.LibraryID)
		if err != nil {
			return ListResult{}, fmt.Errorf("count series by library: %w", err)
		}
		rows, err = s.q.ListSeriesByLibrary(ctx, dbsqlite.ListSeriesByLibraryParams{
			LibraryID: req.LibraryID,
			Limit:     limit,
			Offset:    offset,
		})
		if err != nil {
			return ListResult{}, fmt.Errorf("list series by library: %w", err)
		}
	} else {
		var err error
		total, err = s.q.CountSeries(ctx)
		if err != nil {
			return ListResult{}, fmt.Errorf("count series: %w", err)
		}
		rows, err = s.q.ListSeries(ctx, dbsqlite.ListSeriesParams{
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			return ListResult{}, fmt.Errorf("list series: %w", err)
		}
	}

	series := make([]Series, 0, len(rows))
	for _, row := range rows {
		sr, err := s.buildSeries(ctx, row)
		if err != nil {
			return ListResult{}, err
		}
		series = append(series, sr)
	}

	return ListResult{
		Series:  series,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	}, nil
}

// Update modifies the mutable fields of a series.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Series, error) {
	existing, err := s.q.GetSeries(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Series{}, ErrNotFound
		}
		return Series{}, fmt.Errorf("get series for update: %w", err)
	}

	title := req.Title
	if title == "" {
		title = existing.Title
	}
	libraryID := req.LibraryID
	if libraryID == "" {
		libraryID = existing.LibraryID
	}
	qualityProfileID := req.QualityProfileID
	if qualityProfileID == "" {
		qualityProfileID = existing.QualityProfileID
	}
	seriesType := req.SeriesType
	if seriesType == "" {
		seriesType = existing.SeriesType
	}

	var pathPtr *string
	if req.Path != "" {
		pathPtr = &req.Path
	} else {
		pathPtr = existing.Path
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.UpdateSeries(ctx, dbsqlite.UpdateSeriesParams{
		ID:               id,
		Title:            title,
		Monitored:        boolToInt(req.Monitored),
		LibraryID:        libraryID,
		QualityProfileID: qualityProfileID,
		SeriesType:       seriesType,
		Path:             pathPtr,
		UpdatedAt:        now,
	})
	if err != nil {
		return Series{}, fmt.Errorf("update series: %w", err)
	}

	return s.buildSeries(ctx, row)
}

// Delete removes a series by ID. Cascade deletes handle seasons and episodes.
func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.q.GetSeries(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get series for delete: %w", err)
	}

	if err := s.q.DeleteSeries(ctx, id); err != nil {
		return fmt.Errorf("delete series: %w", err)
	}

	if s.bus != nil {
		s.bus.Publish(ctx, events.Event{
			Type:   events.TypeShowDeleted,
			ShowID: id,
		})
	}

	return nil
}

// Lookup searches the metadata provider without adding anything to the library.
func (s *Service) Lookup(ctx context.Context, req LookupRequest) ([]tmdbtv.SearchResult, error) {
	if s.meta == nil {
		return nil, ErrMetadataNotConfigured
	}

	if req.TMDBID != 0 {
		detail, err := s.meta.GetSeries(ctx, req.TMDBID)
		if err != nil {
			return nil, fmt.Errorf("fetch series by TMDB ID: %w", err)
		}
		return []tmdbtv.SearchResult{{
			ID:           detail.ID,
			Title:        detail.Title,
			Overview:     detail.Overview,
			FirstAirDate: detail.FirstAirDate,
			Year:         detail.Year,
			PosterPath:   detail.PosterPath,
			BackdropPath: detail.BackdropPath,
		}}, nil
	}

	results, err := s.meta.SearchSeries(ctx, req.Query, req.Year)
	if err != nil {
		return nil, fmt.Errorf("search series: %w", err)
	}
	return results, nil
}

// GetSeasons returns all seasons for the given series ID.
func (s *Service) GetSeasons(ctx context.Context, seriesID string) ([]Season, error) {
	rows, err := s.q.ListSeasonsBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list seasons: %w", err)
	}
	seasons := make([]Season, len(rows))
	for i, r := range rows {
		seasons[i] = rowToSeason(r)
	}
	return seasons, nil
}

// GetEpisodes returns all episodes for the given season ID.
func (s *Service) GetEpisodes(ctx context.Context, seasonID string) ([]Episode, error) {
	rows, err := s.q.ListEpisodesBySeasonID(ctx, seasonID)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}
	episodes := make([]Episode, len(rows))
	for i, r := range rows {
		episodes[i] = rowToEpisode(r)
	}
	return episodes, nil
}

// UpdateEpisodeMonitored sets the monitored flag on a single episode.
func (s *Service) UpdateEpisodeMonitored(ctx context.Context, episodeID string, monitored bool) error {
	if err := s.q.UpdateEpisodeMonitored(ctx, dbsqlite.UpdateEpisodeMonitoredParams{
		Monitored: boolToInt(monitored),
		ID:        episodeID,
	}); err != nil {
		return fmt.Errorf("update episode monitored: %w", err)
	}
	return nil
}

// UpdateSeasonMonitored sets the monitored flag on a season and cascades the
// same value to all episodes in that season.
func (s *Service) UpdateSeasonMonitored(ctx context.Context, seasonID string, monitored bool) error {
	monInt := boolToInt(monitored)

	if err := s.q.UpdateSeasonMonitored(ctx, dbsqlite.UpdateSeasonMonitoredParams{
		Monitored: monInt,
		ID:        seasonID,
	}); err != nil {
		return fmt.Errorf("update season monitored: %w", err)
	}

	if err := s.q.UpdateEpisodesMonitoredBySeason(ctx, dbsqlite.UpdateEpisodesMonitoredBySeasonParams{
		Monitored: monInt,
		SeasonID:  seasonID,
	}); err != nil {
		return fmt.Errorf("cascade episode monitored for season: %w", err)
	}

	return nil
}

// buildSeries converts a DB row into a domain Series, fetching episode counts.
func (s *Service) buildSeries(ctx context.Context, row dbsqlite.Series) (Series, error) {
	sr, err := rowToSeries(row)
	if err != nil {
		return Series{}, err
	}

	epCount, err := s.q.CountEpisodesBySeriesID(ctx, row.ID)
	if err != nil {
		return Series{}, fmt.Errorf("count episodes: %w", err)
	}
	fileCount, err := s.q.CountEpisodesWithFileBySeriesID(ctx, row.ID)
	if err != nil {
		return Series{}, fmt.Errorf("count episode files: %w", err)
	}

	sr.EpisodeCount = epCount
	sr.EpisodeFileCount = fileCount
	return sr, nil
}

// rowToSeries converts a dbsqlite.Series row into a domain Series.
// It unmarshals genres_json and parses timestamp strings.
func rowToSeries(row dbsqlite.Series) (Series, error) {
	var genres []string
	if row.GenresJson != "" && row.GenresJson != "null" {
		if err := json.Unmarshal([]byte(row.GenresJson), &genres); err != nil {
			return Series{}, fmt.Errorf("unmarshal genres_json: %w", err)
		}
	}

	addedAt, err := time.Parse(time.RFC3339, row.AddedAt)
	if err != nil {
		addedAt = time.Time{}
	}
	updatedAt, err := time.Parse(time.RFC3339, row.UpdatedAt)
	if err != nil {
		updatedAt = time.Time{}
	}

	var metaRefreshed *time.Time
	if row.MetadataRefreshedAt != nil && *row.MetadataRefreshedAt != "" {
		t, err := time.Parse(time.RFC3339, *row.MetadataRefreshedAt)
		if err == nil {
			metaRefreshed = &t
		}
	}

	var imdbID string
	if row.ImdbID != nil {
		imdbID = *row.ImdbID
	}
	var runtimeMinutes int
	if row.RuntimeMinutes != nil {
		runtimeMinutes = int(*row.RuntimeMinutes)
	}
	var posterURL string
	if row.PosterUrl != nil {
		posterURL = *row.PosterUrl
	}
	var fanartURL string
	if row.FanartUrl != nil {
		fanartURL = *row.FanartUrl
	}
	var network string
	if row.Network != nil {
		network = *row.Network
	}
	var airTime string
	if row.AirTime != nil {
		airTime = *row.AirTime
	}
	var certification string
	if row.Certification != nil {
		certification = *row.Certification
	}
	var path string
	if row.Path != nil {
		path = *row.Path
	}

	return Series{
		ID:                  row.ID,
		TMDBID:              int(row.TmdbID),
		IMDBID:              imdbID,
		Title:               row.Title,
		SortTitle:           row.SortTitle,
		Year:                int(row.Year),
		Overview:            row.Overview,
		RuntimeMinutes:      runtimeMinutes,
		Genres:              genres,
		PosterURL:           posterURL,
		FanartURL:           fanartURL,
		Status:              row.Status,
		SeriesType:          row.SeriesType,
		MonitorType:         row.MonitorType,
		Network:             network,
		AirTime:             airTime,
		Certification:       certification,
		Monitored:           row.Monitored != 0,
		LibraryID:           row.LibraryID,
		QualityProfileID:    row.QualityProfileID,
		Path:                path,
		AddedAt:             addedAt,
		UpdatedAt:           updatedAt,
		MetadataRefreshedAt: metaRefreshed,
	}, nil
}

// rowToSeason converts a dbsqlite.Season row into a domain Season.
func rowToSeason(row dbsqlite.Season) Season {
	return Season{
		ID:           row.ID,
		SeriesID:     row.SeriesID,
		SeasonNumber: int(row.SeasonNumber),
		Monitored:    row.Monitored != 0,
	}
}

// rowToEpisode converts a dbsqlite.Episode row into a domain Episode.
func rowToEpisode(row dbsqlite.Episode) Episode {
	var absNum *int
	if row.AbsoluteNumber != nil {
		n := int(*row.AbsoluteNumber)
		absNum = &n
	}
	var airDate string
	if row.AirDate != nil {
		airDate = *row.AirDate
	}
	return Episode{
		ID:             row.ID,
		SeriesID:       row.SeriesID,
		SeasonID:       row.SeasonID,
		SeasonNumber:   int(row.SeasonNumber),
		EpisodeNumber:  int(row.EpisodeNumber),
		AbsoluteNumber: absNum,
		AirDate:        airDate,
		Title:          row.Title,
		Overview:       row.Overview,
		Monitored:      row.Monitored != 0,
		HasFile:        row.HasFile != 0,
	}
}

// sortTitle returns a normalised sort key by stripping common leading articles.
func sortTitle(title string) string {
	lower := strings.ToLower(title)
	for _, prefix := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, prefix) {
			return strings.ToLower(title[len(prefix):])
		}
	}
	return lower
}

// boolToInt converts a bool to an SQLite integer (0 or 1).
func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// nullableString returns nil for an empty string, otherwise a pointer to the value.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ── Episode file methods ──────────────────────────────────────────────────────

// ListFiles returns all episode files associated with the given series ID.
func (s *Service) ListFiles(ctx context.Context, seriesID string) ([]dbsqlite.EpisodeFile, error) {
	files, err := s.q.ListEpisodeFilesBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list episode files for series %q: %w", seriesID, err)
	}
	return files, nil
}

// DeleteFile removes an episode_file record.  When deleteFromDisk is true it
// also removes the underlying file from the filesystem and marks the episode as
// no longer having a file.
func (s *Service) DeleteFile(ctx context.Context, fileID string, deleteFromDisk bool) error {
	f, err := s.q.GetEpisodeFile(ctx, fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFileNotFound
		}
		return fmt.Errorf("get episode file %q: %w", fileID, err)
	}

	if deleteFromDisk {
		if rmErr := os.Remove(f.Path); rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("remove file from disk %q: %w", f.Path, rmErr)
		}
	}

	if err := s.q.DeleteEpisodeFile(ctx, fileID); err != nil {
		return fmt.Errorf("delete episode_file record %q: %w", fileID, err)
	}

	// Clear has_file on the associated episode.
	ep, err := s.q.GetEpisode(ctx, f.EpisodeID)
	if err == nil {
		_, _ = s.q.UpdateEpisode(ctx, dbsqlite.UpdateEpisodeParams{
			ID:       ep.ID,
			Title:    ep.Title,
			Overview: ep.Overview,
			AirDate:  ep.AirDate,
			HasFile:  0,
		})
	}

	return nil
}

// RenameFiles computes (and optionally applies) renames for all episode files
// belonging to a series.  This is a stub: when no renamer is configured it
// returns an empty slice.  A full implementation will be wired in once the
// renamer package is available.
func (s *Service) RenameFiles(ctx context.Context, seriesID string, dryRun bool) ([]RenamePreview, error) {
	// Validate that the series exists.
	if _, err := s.Get(ctx, seriesID); err != nil {
		return nil, err
	}
	// Full rename logic is added in a follow-up when the renamer package lands.
	// For now return an empty list so the API endpoint is functional.
	return nil, nil
}

// ListAllTMDBIDs returns all TMDB IDs of series in the library.
// Used for "already added" detection in the Discover UI.
func (s *Service) ListAllTMDBIDs(ctx context.Context) ([]int64, error) {
	return s.q.ListAllTMDBIDs(ctx)
}
