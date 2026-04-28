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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/renamer"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// RenameSettings holds the format configuration for renaming files.
type RenameSettings struct {
	EpisodeFormat      string
	SeriesFolderFormat string
	SeasonFolderFormat string
	ColonReplacement   renamer.ColonReplacement
}

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
	GetAlternativeTitles(ctx context.Context, tmdbID int) ([]string, error)
}

// AnimeLookup answers "is this TMDB tv id an anime entry?" — used to
// auto-flag SeriesType=anime and to drive absolute-episode-number
// population during series add/refresh. The interface decouples the
// show service from the animelist package's concrete Service so unit
// tests can stub in a fake without touching the network.
//
// TVDBSeasonToAbsolute translates a TVDB-tagged (season, episode) to
// the show's absolute episode number using the Anime-Lists XML
// mapping data. Used by the search filter to accept fansub releases
// tagged "Show S03E01" as the user's TMDB-relative S01E48. Returns
// (0, false) when conversion isn't possible (non-anime tmdb id,
// unmapped season, etc).
//
// CourBounds returns every cour the show is split into per the
// Anime-Lists XML, sorted ascending by tvdb_season. The result drives
// the cour-shaped Series Detail view for anime — TMDB serves multi-cour
// shows like Jujutsu Kaisen as a single "Season 1" with 59 episodes,
// but the user expects three seasons. Returns an empty slice when the
// series has no Anime-Lists mapping; callers fall back to the regular
// TMDB-shape view.
type AnimeLookup interface {
	IsAnime(tmdbID int) bool
	TVDBSeasonToAbsolute(tmdbID, tvdbSeason, tvdbEpisode int) (int, bool)
	CourBounds(tmdbID int) []CourBound
}

// CourBound describes a single cour's slot within the show's TMDB
// layout. (tmdb_season, tmdb_offset+1) is the first TMDB-relative
// episode of the cour; the cour ends at the start of the next cour
// in tvdb_season order, or at the end of the season for the last cour.
type CourBound struct {
	TVDBSeason int    // cour identifier (1, 2, 3, …) — matches Anime-Lists' defaulttvdbseason
	TMDBSeason int    // which TMDB season this cour falls inside (almost always 1)
	TMDBOffset int    // count of TMDB episodes in earlier cours of the same TMDB season
	Name       string // AniDB-supplied cour name, e.g. "Jujutsu Kaisen Season 2"
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
	// AlternateTitles are alternate marketing/translated names for the
	// series (e.g. "Star Wars: Andor" for "Andor"), fetched from TMDB.
	// Used by parser.TitleMatchesAny so indexer responses with non-
	// canonical names aren't dropped by the strict series-title gate.
	AlternateTitles []string
}

// Season is the domain representation of a season record.
type Season struct {
	ID               string
	SeriesID         string
	SeasonNumber     int
	Monitored        bool
	EpisodeCount     int64
	EpisodeFileCount int64
	TotalSizeBytes   int64
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
	StillPath      string
	RuntimeMinutes int
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
	row      db.Season
	episodes []db.Episode
}

// Service manages TV series, seasons, and episodes.
type Service struct {
	q      db.Querier
	meta   MetadataProvider
	anime  AnimeLookup
	bus    *events.Bus
	logger *slog.Logger
}

// NewService creates a new Service. meta may be nil; methods that require it
// will return ErrMetadataNotConfigured. anime may be nil; when nil, no anime
// auto-detection happens (series add/refresh leaves SeriesType as the
// caller specified and absolute_number unset).
func NewService(q db.Querier, meta MetadataProvider, anime AnimeLookup, bus *events.Bus, logger *slog.Logger) *Service {
	return &Service{
		q:      q,
		meta:   meta,
		anime:  anime,
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
	_, err := s.q.GetSeriesByTMDBID(ctx, int32(req.TMDBID))
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

	// Fetch alternate titles best-effort. Failure here doesn't abort the
	// add — the series still works, just with stricter title matching
	// until a refresh-metadata call succeeds. Most TMDB shows have at
	// least one alternate (regional/marketing name); for series that
	// don't, the empty list is fine.
	altTitles, altErr := s.meta.GetAlternativeTitles(ctx, req.TMDBID)
	if altErr != nil {
		s.logger.Warn("series add: alternate titles fetch failed (continuing)",
			"tmdb_id", req.TMDBID, "error", altErr)
		altTitles = nil
	}
	altTitlesJSON, err := json.Marshal(altTitles)
	if err != nil || altTitles == nil {
		altTitlesJSON = []byte("[]")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	nowTime := time.Now().UTC()
	metaNow := nowTime.Format(time.RFC3339)

	genresJSON, err := json.Marshal(detail.Genres)
	if err != nil {
		return Series{}, fmt.Errorf("marshal genres: %w", err)
	}

	seriesID := uuid.NewString()

	monitorType := req.MonitorType
	if monitorType == "" {
		monitorType = "all"
	}
	seriesType := req.SeriesType
	if seriesType == "" {
		seriesType = "standard"
	}
	// Auto-flag anime when the show appears in the Anime-Lists XML.
	// Caller-supplied "anime" or "daily" is preserved (explicit > inferred);
	// only the default "standard" is upgraded. This is what powers the
	// anime-aware search-query expansion downstream — without it, episode
	// search for shows whose fansub releases use absolute numbering
	// ("Show - 48") returns zero results because we'd only emit S01E48.
	if seriesType == "standard" && s.anime != nil && s.anime.IsAnime(req.TMDBID) {
		seriesType = "anime"
		s.logger.Info("series add: auto-detected as anime via anime-list",
			"tmdb_id", req.TMDBID, "title", detail.Title)
	}

	runtimeMinutes := sql.NullInt32{}
	if detail.RuntimeMinutes > 0 {
		runtimeMinutes = sql.NullInt32{Int32: int32(detail.RuntimeMinutes), Valid: true}
	}

	posterURL := toNullString(detail.PosterPath)
	fanartURL := toNullString(detail.BackdropPath)
	network := toNullString(detail.Network)

	row, err := s.q.CreateSeries(ctx, db.CreateSeriesParams{
		ID:                  seriesID,
		TmdbID:              int32(detail.ID),
		ImdbID:              sql.NullString{},
		Title:               detail.Title,
		SortTitle:           sortTitle(detail.Title),
		Year:                int32(detail.Year),
		Overview:            detail.Overview,
		RuntimeMinutes:      runtimeMinutes,
		GenresJson:          string(genresJSON),
		PosterUrl:           posterURL,
		FanartUrl:           fanartURL,
		Status:              detail.Status,
		SeriesType:          seriesType,
		MonitorType:         monitorType,
		Network:             network,
		AirTime:             sql.NullString{},
		Certification:       sql.NullString{},
		Monitored:           req.Monitored,
		LibraryID:           req.LibraryID,
		QualityProfileID:    req.QualityProfileID,
		Path:                sql.NullString{},
		AddedAt:             now,
		UpdatedAt:           now,
		MetadataRefreshedAt: sql.NullString{String: metaNow, Valid: true},
		AlternateTitles:     altTitlesJSON,
	})
	if err != nil {
		return Series{}, fmt.Errorf("create series: %w", err)
	}

	// Create seasons and episodes. Season 0 (specials) is included if present.
	// We track season rows by season_number so we can apply monitor_type later.
	seasonMap := make(map[int]seasonEntry, len(detail.Seasons))

	// Anime needs absolute episode numbering for indexer queries — fansub
	// releases tag episodes as "Show - 48" rather than "S01E48". Iterate
	// seasons in numerical order and accumulate a running absolute counter
	// across non-zero seasons. Season 0 (specials) doesn't participate.
	// For shows TMDB serves as one big season (Jujutsu Kaisen) this just
	// makes absolute = episode_number; for split shows it produces the
	// correct cumulative count.
	sortedSeasons := append([]tmdbtv.SeasonSummary(nil), detail.Seasons...)
	sort.Slice(sortedSeasons, func(i, j int) bool {
		return sortedSeasons[i].SeasonNumber < sortedSeasons[j].SeasonNumber
	})
	isAnime := seriesType == "anime"
	absoluteCounter := 0

	for _, ss := range sortedSeasons {
		seasonID := uuid.NewString()
		seasonRow, err := s.q.CreateSeason(ctx, db.CreateSeasonParams{
			ID:           seasonID,
			SeriesID:     seriesID,
			SeasonNumber: int32(ss.SeasonNumber),
			Monitored:    true, // will be overridden by monitor_type pass below
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

		// Sort episodes within the season by episode_number so the
		// running absolute counter accumulates in the right order even
		// if TMDB returns them out-of-order.
		sort.SliceStable(epDetails, func(i, j int) bool {
			return epDetails[i].EpisodeNumber < epDetails[j].EpisodeNumber
		})

		var episodeRows []db.Episode
		for _, ep := range epDetails {
			epID := uuid.NewString()
			airDate := toNullString(ep.AirDate)
			absolute := sql.NullInt32{}
			if isAnime && ep.SeasonNumber > 0 {
				absoluteCounter++
				absolute = sql.NullInt32{Int32: int32(absoluteCounter), Valid: true}
			}
			epRow, err := s.q.CreateEpisode(ctx, db.CreateEpisodeParams{
				ID:             epID,
				SeriesID:       seriesID,
				SeasonID:       seasonID,
				SeasonNumber:   int32(ep.SeasonNumber),
				EpisodeNumber:  int32(ep.EpisodeNumber),
				AbsoluteNumber: absolute,
				AirDate:        airDate,
				Title:          ep.Title,
				Overview:       ep.Overview,
				Monitored:      true, // will be overridden by monitor_type pass below
				HasFile:        false,
				StillPath:      ep.StillPath,
				RuntimeMinutes: int32(ep.RuntimeMinutes),
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
			var monitored bool

			switch monitorType {
			case "all":
				monitored = true

			case "future":
				// Monitor episodes with no air date or air date after today.
				if !ep.AirDate.Valid || ep.AirDate.String == "" || ep.AirDate.String > today {
					monitored = true
				}

			case "missing":
				// For a new series, no files exist — monitor all aired episodes.
				if ep.AirDate.Valid && ep.AirDate.String != "" && ep.AirDate.String <= today {
					monitored = true
				}

			case "none":
				monitored = false

			case "pilot":
				if seasonNum == 1 && ep.EpisodeNumber == 1 {
					monitored = true
				}

			case "first_season":
				if seasonNum == 1 {
					monitored = true
				}

			case "last_season":
				if seasonNum == lastSeasonNum {
					monitored = true
				}

			case "existing":
				// New series has no files — nothing is monitored.
				monitored = false

			default:
				monitored = true
			}

			if err := s.q.UpdateEpisodeMonitored(ctx, db.UpdateEpisodeMonitoredParams{
				Monitored: monitored,
				ID:        ep.ID,
			}); err != nil {
				return fmt.Errorf("update episode monitored %s: %w", ep.ID, err)
			}
		}

		// Season is monitored if any episode within it is monitored.
		seasonMonitored := false
		switch monitorType {
		case "all":
			seasonMonitored = true
		case "future", "missing":
			seasonMonitored = true
		case "first_season":
			if seasonNum == 1 {
				seasonMonitored = true
			}
		case "last_season":
			if seasonNum == lastSeasonNum {
				seasonMonitored = true
			}
		case "pilot":
			if seasonNum == 1 {
				seasonMonitored = true
			}
		case "none", "existing":
			seasonMonitored = false
		default:
			seasonMonitored = true
		}

		if err := s.q.UpdateSeasonMonitored(ctx, db.UpdateSeasonMonitoredParams{
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

	offset := int32((page - 1) * perPage)
	limit := int32(perPage)

	var rows []db.Series
	var total int64

	if req.LibraryID != "" {
		var err error
		total, err = s.q.CountSeriesByLibrary(ctx, req.LibraryID)
		if err != nil {
			return ListResult{}, fmt.Errorf("count series by library: %w", err)
		}
		rows, err = s.q.ListSeriesByLibrary(ctx, db.ListSeriesByLibraryParams{
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
		rows, err = s.q.ListSeries(ctx, db.ListSeriesParams{
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

	pathVal := existing.Path
	if req.Path != "" {
		pathVal = sql.NullString{String: req.Path, Valid: true}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.UpdateSeries(ctx, db.UpdateSeriesParams{
		ID:               id,
		Title:            title,
		Monitored:        req.Monitored,
		LibraryID:        libraryID,
		QualityProfileID: qualityProfileID,
		SeriesType:       seriesType,
		Path:             pathVal,
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

// GetSeasons returns all seasons for the given series ID, annotated with
// per-season episode counts (total and with-file).
func (s *Service) GetSeasons(ctx context.Context, seriesID string) ([]Season, error) {
	rows, err := s.q.ListSeasonsBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list seasons: %w", err)
	}
	seasons := make([]Season, len(rows))
	for i, r := range rows {
		seasons[i] = rowToSeason(r)
	}

	episodes, err := s.q.ListEpisodesBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list episodes for season counts: %w", err)
	}
	totals := make(map[string]int64, len(seasons))
	withFile := make(map[string]int64, len(seasons))
	episodeToSeason := make(map[string]string, len(episodes))
	for _, ep := range episodes {
		totals[ep.SeasonID]++
		if ep.HasFile {
			withFile[ep.SeasonID]++
		}
		episodeToSeason[ep.ID] = ep.SeasonID
	}

	files, err := s.q.ListEpisodeFilesBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("list episode files for season sizes: %w", err)
	}
	sizes := make(map[string]int64, len(seasons))
	for _, f := range files {
		if seasonID, ok := episodeToSeason[f.EpisodeID]; ok {
			sizes[seasonID] += f.SizeBytes
		}
	}

	for i := range seasons {
		seasons[i].EpisodeCount = totals[seasons[i].ID]
		seasons[i].EpisodeFileCount = withFile[seasons[i].ID]
		seasons[i].TotalSizeBytes = sizes[seasons[i].ID]
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
	if err := s.q.UpdateEpisodeMonitored(ctx, db.UpdateEpisodeMonitoredParams{
		Monitored: monitored,
		ID:        episodeID,
	}); err != nil {
		return fmt.Errorf("update episode monitored: %w", err)
	}
	return nil
}

// UpdateSeasonMonitored sets the monitored flag on a season and cascades the
// same value to all episodes in that season.
func (s *Service) UpdateSeasonMonitored(ctx context.Context, seasonID string, monitored bool) error {
	if err := s.q.UpdateSeasonMonitored(ctx, db.UpdateSeasonMonitoredParams{
		Monitored: monitored,
		ID:        seasonID,
	}); err != nil {
		return fmt.Errorf("update season monitored: %w", err)
	}

	if err := s.q.UpdateEpisodesMonitoredBySeason(ctx, db.UpdateEpisodesMonitoredBySeasonParams{
		Monitored: monitored,
		SeasonID:  seasonID,
	}); err != nil {
		return fmt.Errorf("cascade episode monitored for season: %w", err)
	}

	return nil
}

// buildSeries converts a DB row into a domain Series, fetching episode counts.
func (s *Service) buildSeries(ctx context.Context, row db.Series) (Series, error) {
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

// rowToSeries converts a db.Series row into a domain Series.
// It unmarshals genres_json and parses timestamp strings.
func rowToSeries(row db.Series) (Series, error) {
	var genres []string
	if row.GenresJson != "" && row.GenresJson != "null" {
		if err := json.Unmarshal([]byte(row.GenresJson), &genres); err != nil {
			return Series{}, fmt.Errorf("unmarshal genres_json: %w", err)
		}
	}

	var alternateTitles []string
	if len(row.AlternateTitles) > 0 && string(row.AlternateTitles) != "null" {
		if err := json.Unmarshal(row.AlternateTitles, &alternateTitles); err != nil {
			// Don't fail series fetch on bad alternates JSON — just log
			// and proceed with empty list. The strict-title gate will
			// still match against the canonical title.
			alternateTitles = nil
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
	if row.MetadataRefreshedAt.Valid && row.MetadataRefreshedAt.String != "" {
		t, err := time.Parse(time.RFC3339, row.MetadataRefreshedAt.String)
		if err == nil {
			metaRefreshed = &t
		}
	}

	var runtimeMinutes int
	if row.RuntimeMinutes.Valid {
		runtimeMinutes = int(row.RuntimeMinutes.Int32)
	}

	return Series{
		ID:                  row.ID,
		TMDBID:              int(row.TmdbID),
		IMDBID:              row.ImdbID.String,
		Title:               row.Title,
		SortTitle:           row.SortTitle,
		Year:                int(row.Year),
		Overview:            row.Overview,
		RuntimeMinutes:      runtimeMinutes,
		Genres:              genres,
		PosterURL:           row.PosterUrl.String,
		FanartURL:           row.FanartUrl.String,
		Status:              row.Status,
		SeriesType:          row.SeriesType,
		MonitorType:         row.MonitorType,
		Network:             row.Network.String,
		AirTime:             row.AirTime.String,
		Certification:       row.Certification.String,
		Monitored:           row.Monitored,
		LibraryID:           row.LibraryID,
		QualityProfileID:    row.QualityProfileID,
		Path:                row.Path.String,
		AddedAt:             addedAt,
		UpdatedAt:           updatedAt,
		MetadataRefreshedAt: metaRefreshed,
		AlternateTitles:     alternateTitles,
	}, nil
}

// rowToSeason converts a db.Season row into a domain Season.
func rowToSeason(row db.Season) Season {
	return Season{
		ID:           row.ID,
		SeriesID:     row.SeriesID,
		SeasonNumber: int(row.SeasonNumber),
		Monitored:    row.Monitored,
	}
}

// rowToEpisode converts a db.Episode row into a domain Episode.
func rowToEpisode(row db.Episode) Episode {
	var absNum *int
	if row.AbsoluteNumber.Valid {
		n := int(row.AbsoluteNumber.Int32)
		absNum = &n
	}
	return Episode{
		ID:             row.ID,
		SeriesID:       row.SeriesID,
		SeasonID:       row.SeasonID,
		SeasonNumber:   int(row.SeasonNumber),
		EpisodeNumber:  int(row.EpisodeNumber),
		AbsoluteNumber: absNum,
		AirDate:        row.AirDate.String,
		Title:          row.Title,
		Overview:       row.Overview,
		Monitored:      row.Monitored,
		HasFile:        row.HasFile,
		StillPath:      row.StillPath,
		RuntimeMinutes: int(row.RuntimeMinutes),
	}
}

// RefreshMetadata re-fetches series-level metadata from TMDB (including
// alternate titles) and updates the row. Episode metadata is NOT
// refreshed here — call RefreshEpisodeMetadata for that.
//
// Used to backfill alternate_titles for series that were added before
// the alternates feature shipped (the column starts as []), and to pick
// up alternate-title additions on TMDB after a series has aged in the
// library.
func (s *Service) RefreshMetadata(ctx context.Context, seriesID string) (Series, error) {
	if s.meta == nil {
		return Series{}, ErrMetadataNotConfigured
	}

	row, err := s.q.GetSeries(ctx, seriesID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Series{}, ErrNotFound
		}
		return Series{}, fmt.Errorf("get series: %w", err)
	}

	detail, err := s.meta.GetSeries(ctx, int(row.TmdbID))
	if err != nil {
		return Series{}, fmt.Errorf("fetch series metadata: %w", err)
	}

	altTitles, altErr := s.meta.GetAlternativeTitles(ctx, int(row.TmdbID))
	if altErr != nil {
		s.logger.Warn("refresh metadata: alternate titles fetch failed (continuing)",
			"series_id", seriesID, "tmdb_id", row.TmdbID, "error", altErr)
		altTitles = nil
	}
	altTitlesJSON, err := json.Marshal(altTitles)
	if err != nil || altTitles == nil {
		altTitlesJSON = []byte("[]")
	}

	genresJSON, err := json.Marshal(detail.Genres)
	if err != nil {
		return Series{}, fmt.Errorf("marshal genres: %w", err)
	}

	runtimeMinutes := sql.NullInt32{}
	if detail.RuntimeMinutes > 0 {
		runtimeMinutes = sql.NullInt32{Int32: int32(detail.RuntimeMinutes), Valid: true}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	updated, err := s.q.UpdateSeriesMetadata(ctx, db.UpdateSeriesMetadataParams{
		ImdbID:              row.ImdbID,
		Title:               detail.Title,
		SortTitle:           sortTitle(detail.Title),
		Year:                int32(detail.Year),
		Overview:            detail.Overview,
		RuntimeMinutes:      runtimeMinutes,
		GenresJson:          string(genresJSON),
		PosterUrl:           toNullString(detail.PosterPath),
		FanartUrl:           toNullString(detail.BackdropPath),
		Status:              detail.Status,
		Network:             toNullString(detail.Network),
		AirTime:             row.AirTime,
		Certification:       row.Certification,
		MetadataRefreshedAt: sql.NullString{String: now, Valid: true},
		UpdatedAt:           now,
		AlternateTitles:     altTitlesJSON,
		ID:                  seriesID,
	})
	if err != nil {
		return Series{}, fmt.Errorf("update series metadata: %w", err)
	}

	if s.BackfillAnimeIfNeeded(ctx, seriesID, int(row.TmdbID), row.SeriesType) {
		updated.SeriesType = "anime"
	}

	s.logger.Info("series metadata refreshed",
		"series_id", seriesID,
		"tmdb_id", row.TmdbID,
		"alternate_titles_count", len(altTitles))

	return rowToSeries(updated)
}

// BackfillAnimeIfNeeded is the standalone anime-detection + backfill
// helper, factored out of RefreshMetadata so it can run independently
// of the (sometimes-unreachable) TMDB metadata refresh path. Returns
// true when the series was upgraded to anime — caller can use this to
// update its in-memory representation; the DB write has already
// happened.
//
// Idempotent: safe to call repeatedly. Skips when:
//   - anime lookup is disabled (s.anime == nil)
//   - the series is already non-standard (anime/daily — caller's
//     explicit choice wins)
//   - the TMDB id isn't in the Anime-Lists XML
//
// Logs all decisions at info level so the startup scan produces an
// auditable trail.
func (s *Service) BackfillAnimeIfNeeded(ctx context.Context, seriesID string, tmdbID int, currentSeriesType string) bool {
	if s.anime == nil || currentSeriesType != "standard" || !s.anime.IsAnime(tmdbID) {
		return false
	}
	// Use the existing UpdateSeries query — it sets series_type along
	// with a few other fields. Re-load the row so we preserve the
	// non-affected ones (title, library, quality profile, monitored,
	// path) verbatim and only flip series_type.
	row, err := s.q.GetSeries(ctx, seriesID)
	if err != nil {
		s.logger.Warn("anime backfill: failed to load series for upgrade",
			"series_id", seriesID, "tmdb_id", tmdbID, "error", err)
		return false
	}
	if _, err := s.q.UpdateSeries(ctx, db.UpdateSeriesParams{
		ID:               seriesID,
		Title:            row.Title,
		Monitored:        row.Monitored,
		LibraryID:        row.LibraryID,
		QualityProfileID: row.QualityProfileID,
		SeriesType:       "anime",
		Path:             row.Path,
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		s.logger.Warn("anime backfill: failed to upgrade series_type",
			"series_id", seriesID, "tmdb_id", tmdbID, "error", err)
		return false
	}
	s.logger.Info("series upgraded to anime via anime-list",
		"series_id", seriesID, "tmdb_id", tmdbID)
	if err := s.populateAbsoluteNumbers(ctx, seriesID); err != nil {
		s.logger.Warn("anime backfill: absolute_number population failed",
			"series_id", seriesID, "error", err)
	}
	return true
}

// BackfillAnimeForAllSeries iterates every standard-typed series in the
// library and runs BackfillAnimeIfNeeded against each. Intended as a
// one-shot startup task: catches series added before anime detection
// existed without requiring the user to manually refresh each one.
//
// Designed to be safe to call concurrently with other Service methods —
// each row is processed independently. Errors on individual rows are
// logged and skipped, never returned (one bad row shouldn't abort the
// whole sweep).
func (s *Service) BackfillAnimeForAllSeries(ctx context.Context) {
	if s.anime == nil {
		return
	}
	// Page through ListSeries to avoid loading the entire library at
	// once — small libraries fit in one page, larger ones don't.
	const pageSize = 200
	scanned, upgraded := 0, 0
	for offset := int32(0); ; offset += pageSize {
		if ctx.Err() != nil {
			return
		}
		rows, err := s.q.ListSeries(ctx, db.ListSeriesParams{Limit: pageSize, Offset: offset})
		if err != nil {
			s.logger.Warn("anime backfill sweep: list series failed", "offset", offset, "error", err)
			return
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			scanned++
			if row.SeriesType != "standard" {
				continue
			}
			if s.BackfillAnimeIfNeeded(ctx, row.ID, int(row.TmdbID), row.SeriesType) {
				upgraded++
			}
		}
		if len(rows) < pageSize {
			break
		}
	}
	if upgraded > 0 {
		s.logger.Info("anime backfill sweep complete",
			"scanned", scanned, "upgraded", upgraded)
	}
}

// TVDBSeasonToAbsolute is a thin pass-through to the configured anime
// lookup. Returns (0, false) when no anime lookup is configured.
// Used by the search filter to accept TVDB-tagged fansub releases
// against a TMDB-relative episode request — see filterByEpisode.
func (s *Service) TVDBSeasonToAbsolute(tmdbID, tvdbSeason, tvdbEpisode int) (int, bool) {
	if s.anime == nil {
		return 0, false
	}
	return s.anime.TVDBSeasonToAbsolute(tmdbID, tvdbSeason, tvdbEpisode)
}

// GetEpisodeAbsoluteNumber returns the absolute episode number for a
// given (seriesID, season, episode) tuple. Returns (0, nil) when the
// episode exists but has no absolute number set (the common case for
// non-anime series), or when the episode isn't found at all. Used by
// the search-query builder to emit anime-style queries
// ("Show - 48", "Show 48") in addition to "S01E48".
func (s *Service) GetEpisodeAbsoluteNumber(ctx context.Context, seriesID string, season, episode int) (int, error) {
	episodes, err := s.q.ListEpisodesBySeriesID(ctx, seriesID)
	if err != nil {
		return 0, err
	}
	for _, e := range episodes {
		if int(e.SeasonNumber) == season && int(e.EpisodeNumber) == episode {
			if e.AbsoluteNumber.Valid {
				return int(e.AbsoluteNumber.Int32), nil
			}
			return 0, nil
		}
	}
	return 0, nil
}

// populateAbsoluteNumbers fills in episode.absolute_number for every
// non-special episode of the given series, using the same running-counter
// rule as the Add() path. Idempotent; safe to call repeatedly.
func (s *Service) populateAbsoluteNumbers(ctx context.Context, seriesID string) error {
	episodes, err := s.q.ListEpisodesBySeriesID(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("list episodes for absolute backfill: %w", err)
	}
	// ListEpisodesBySeriesID is already ordered by season_number,
	// episode_number — the running counter accumulates correctly.
	counter := 0
	updated := 0
	for _, ep := range episodes {
		if ep.SeasonNumber == 0 {
			continue // specials don't get absolute numbers
		}
		counter++
		want := int32(counter)
		if ep.AbsoluteNumber.Valid && ep.AbsoluteNumber.Int32 == want {
			continue // already correct, skip the write
		}
		if err := s.q.UpdateEpisodeAbsoluteNumber(ctx, db.UpdateEpisodeAbsoluteNumberParams{
			AbsoluteNumber: sql.NullInt32{Int32: want, Valid: true},
			ID:             ep.ID,
		}); err != nil {
			return fmt.Errorf("update absolute_number for episode %s: %w", ep.ID, err)
		}
		updated++
	}
	if updated > 0 {
		s.logger.Info("absolute_number backfill complete",
			"series_id", seriesID, "updated", updated, "total", len(episodes))
	}
	return nil
}

// RefreshEpisodeMetadata re-fetches episode details from TMDB for the given
// series and updates still_path and runtime_minutes on each episode.
func (s *Service) RefreshEpisodeMetadata(ctx context.Context, seriesID string, tmdbID int) error {
	if s.meta == nil {
		return ErrMetadataNotConfigured
	}

	episodes, err := s.q.ListEpisodesBySeriesID(ctx, seriesID)
	if err != nil {
		return err
	}

	// Group episodes by season to minimise TMDB calls.
	type seasonKey int
	seasonEps := make(map[seasonKey][]db.Episode)
	for _, ep := range episodes {
		sn := seasonKey(ep.SeasonNumber)
		seasonEps[sn] = append(seasonEps[sn], ep)
	}

	for sn, eps := range seasonEps {
		details, err := s.meta.GetSeasonEpisodes(ctx, tmdbID, int(sn))
		if err != nil {
			s.logger.Warn("refresh: failed to fetch season episodes",
				"tmdb_id", tmdbID, "season", sn, "error", err)
			continue
		}

		detailMap := make(map[int]tmdbtv.EpisodeDetail, len(details))
		for _, d := range details {
			detailMap[d.EpisodeNumber] = d
		}

		for _, ep := range eps {
			d, ok := detailMap[int(ep.EpisodeNumber)]
			if !ok {
				continue
			}
			// Only update if there's new data.
			if ep.StillPath == d.StillPath && ep.RuntimeMinutes == int32(d.RuntimeMinutes) {
				continue
			}
			_, _ = s.q.UpdateEpisode(ctx, db.UpdateEpisodeParams{
				ID:             ep.ID,
				Title:          ep.Title,
				Overview:       ep.Overview,
				AirDate:        ep.AirDate,
				HasFile:        ep.HasFile,
				StillPath:      d.StillPath,
				RuntimeMinutes: int32(d.RuntimeMinutes),
			})
		}
	}
	return nil
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

// toNullString returns a sql.NullString; Valid is false for an empty string.
func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// ── Episode file methods ──────────────────────────────────────────────────────

// ListFiles returns all episode files associated with the given series ID.
func (s *Service) ListFiles(ctx context.Context, seriesID string) ([]db.EpisodeFile, error) {
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
		_, _ = s.q.UpdateEpisode(ctx, db.UpdateEpisodeParams{
			ID:             ep.ID,
			Title:          ep.Title,
			Overview:       ep.Overview,
			AirDate:        ep.AirDate,
			HasFile:        false,
			StillPath:      ep.StillPath,
			RuntimeMinutes: ep.RuntimeMinutes,
		})
	}

	return nil
}

// RenameFiles computes (and optionally applies) renames for all episode files
// belonging to a series based on the naming format settings.
func (s *Service) RenameFiles(ctx context.Context, seriesID string, settings RenameSettings, dryRun bool) ([]RenamePreview, error) {
	series, err := s.Get(ctx, seriesID)
	if err != nil {
		return nil, err
	}

	lib, err := s.q.GetLibrary(ctx, series.LibraryID)
	if err != nil {
		return nil, fmt.Errorf("loading library: %w", err)
	}

	files, err := s.q.ListEpisodeFilesBySeriesID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("listing episode files: %w", err)
	}

	if len(files) == 0 {
		return nil, nil
	}

	rSeries := renamer.Series{Title: series.Title, Year: int(series.Year)}
	colon := settings.ColonReplacement

	var previews []RenamePreview
	var renameErrors []string

	for _, f := range files {
		// Look up the episode for this file.
		ep, epErr := s.q.GetEpisode(ctx, f.EpisodeID)
		if epErr != nil {
			s.logger.Warn("rename: episode not found for file", "file_id", f.ID, "episode_id", f.EpisodeID)
			continue
		}

		ext := filepath.Ext(f.Path)
		airDate := ep.AirDate.String

		newPath := renamer.DestPath(
			lib.RootPath,
			settings.EpisodeFormat,
			settings.SeriesFolderFormat,
			settings.SeasonFolderFormat,
			rSeries,
			renamer.Episode{
				SeasonNumber:  int(ep.SeasonNumber),
				EpisodeNumber: int(ep.EpisodeNumber),
				Title:         ep.Title,
				AirDate:       airDate,
			},
			plugin.Quality{}, // Quality is not re-parsed for rename; filename is clean already.
			colon,
			ext,
		)

		if newPath == f.Path {
			continue // already correct
		}

		previews = append(previews, RenamePreview{
			FileID:  f.ID,
			OldPath: f.Path,
			NewPath: newPath,
		})

		if !dryRun {
			// Create destination directory.
			if mkErr := os.MkdirAll(filepath.Dir(newPath), 0o755); mkErr != nil {
				renameErrors = append(renameErrors, fmt.Sprintf("mkdir %s: %v", filepath.Dir(newPath), mkErr))
				continue
			}

			// Check target doesn't exist.
			if _, statErr := os.Stat(newPath); statErr == nil {
				renameErrors = append(renameErrors, fmt.Sprintf("target already exists: %s", newPath))
				continue
			}

			if renameErr := os.Rename(f.Path, newPath); renameErr != nil {
				renameErrors = append(renameErrors, fmt.Sprintf("rename %s → %s: %v", f.Path, newPath, renameErr))
				continue
			}

			// Update path in DB.
			if dbErr := s.q.UpdateEpisodeFilePath(ctx, db.UpdateEpisodeFilePathParams{
				Path: newPath,
				ID:   f.ID,
			}); dbErr != nil {
				renameErrors = append(renameErrors, fmt.Sprintf("db update %s: %v", f.ID, dbErr))
			}

			s.logger.Info("renamed episode file", "old", f.Path, "new", newPath)
		}
	}

	if len(renameErrors) > 0 {
		return previews, fmt.Errorf("rename errors: %s", strings.Join(renameErrors, "; "))
	}
	return previews, nil
}

// ListAllTMDBIDs returns all TMDB IDs of series in the library.
// Used for "already added" detection in the Discover UI.
func (s *Service) ListAllTMDBIDs(ctx context.Context) ([]int64, error) {
	ids, err := s.q.ListAllTMDBIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out, nil
}
