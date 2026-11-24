// Package show manages TV series records in the Pilot library.
package show

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
)

// Sentinel errors returned by Service methods.
var (
	ErrNotFound              = errors.New("series not found")
	ErrAlreadyExists         = errors.New("series already in library")
	ErrMetadataNotConfigured = errors.New("metadata provider not configured")
	ErrFileNotFound          = errors.New("episode file not found")
)

// MetadataProvider fetches TV series metadata from an external source.
type MetadataProvider interface {
	SearchSeries(ctx context.Context, query string, year int) ([]tmdbtv.SearchResult, error)
	GetSeries(ctx context.Context, tmdbID int) (*tmdbtv.SeriesDetail, error)
	GetSeriesExtended(ctx context.Context, tmdbID int) (*tmdbtv.SeriesDetail, error)
	GetPerson(ctx context.Context, personID int) (*tmdbtv.Person, error)
	GetPersonTVCredits(ctx context.Context, personID int, personType string) ([]tmdbtv.PersonCredit, error)
	GetSeasonEpisodes(ctx context.Context, tmdbID int, seasonNum int) ([]tmdbtv.EpisodeDetail, error)
	DiscoverTVByGenre(ctx context.Context, genreID, page int) (*tmdbtv.PaginatedSeries, error)
	GetTVGenres(ctx context.Context) ([]tmdbtv.Genre, error)
	Browse(ctx context.Context, f tmdbtv.BrowseFilter) (*tmdbtv.PaginatedSeries, error)
	WatchProviders(ctx context.Context, region string) ([]tmdbtv.WatchProvider, error)
	SearchPaginated(ctx context.Context, query string, page int) (*tmdbtv.PaginatedSeries, error)
	GetAlternativeTitles(ctx context.Context, tmdbID int) ([]string, error)
}

// Series is the domain representation of a TV series record.
type Series struct {
	ID     string
	TMDBID int
	IMDBID string
	// TVDBID is TheTVDB's id, which most indexers match shows by. 0 when
	// unknown (not looked up yet, or TMDB has none).
	TVDBID              int
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
	q       db.Querier
	meta    MetadataProvider
	anime   AnimeLookup
	bus     *events.Bus
	logger  *slog.Logger
	recycle Recycler
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

	var runtimeMinutes *int64
	if detail.RuntimeMinutes > 0 {
		v := int64(detail.RuntimeMinutes)
		runtimeMinutes = &v
	}

	posterURL := toNullString(detail.PosterPath)
	fanartURL := toNullString(detail.BackdropPath)
	network := toNullString(detail.Network)

	// Resolve the quality profile. The Add form's "Any Quality" option
	// (and any caller that omits the field) sends an empty
	// quality_profile_id, but series.quality_profile_id is NOT NULL with a
	// FK to quality_profiles(id) — inserting "" violates the constraint and
	// surfaces as a foreign-key error in the UI. Fall back to the library's
	// configured default profile, which the schema guarantees is a valid row.
	qualityProfileID := req.QualityProfileID
	if qualityProfileID == "" {
		lib, err := s.q.GetLibrary(ctx, req.LibraryID)
		if err != nil {
			return Series{}, fmt.Errorf("resolve default quality profile: %w", err)
		}
		qualityProfileID = lib.DefaultQualityProfileID
	}

	row, err := s.q.CreateSeries(ctx, db.CreateSeriesParams{
		ID:                  seriesID,
		TmdbID:              int64(detail.ID),
		ImdbID:              &detail.IMDBID, // "" = TMDB has none
		TvdbID:              ptrInt64(detail.TVDBID),
		Title:               detail.Title,
		SortTitle:           sortTitle(detail.Title),
		Year:                int64(detail.Year),
		Overview:            detail.Overview,
		RuntimeMinutes:      runtimeMinutes,
		GenresJson:          string(genresJSON),
		PosterUrl:           posterURL,
		FanartUrl:           fanartURL,
		Status:              detail.Status,
		SeriesType:          seriesType,
		MonitorType:         monitorType,
		Network:             network,
		AirTime:             nil,
		Certification:       nil,
		Monitored:           req.Monitored,
		LibraryID:           req.LibraryID,
		QualityProfileID:    qualityProfileID,
		Path:                nil,
		AddedAt:             now,
		UpdatedAt:           now,
		MetadataRefreshedAt: &metaNow,
		AlternateTitles:     string(altTitlesJSON),
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
			SeasonNumber: int64(ss.SeasonNumber),
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
			var absolute *int64
			if isAnime && ep.SeasonNumber > 0 {
				absoluteCounter++
				v := int64(absoluteCounter)
				absolute = &v
			}
			epRow, err := s.q.CreateEpisode(ctx, db.CreateEpisodeParams{
				ID:             epID,
				SeriesID:       seriesID,
				SeasonID:       seasonID,
				SeasonNumber:   int64(ep.SeasonNumber),
				EpisodeNumber:  int64(ep.EpisodeNumber),
				AbsoluteNumber: absolute,
				AirDate:        airDate,
				Title:          ep.Title,
				Overview:       ep.Overview,
				Monitored:      true, // will be overridden by monitor_type pass below
				HasFile:        false,
				StillPath:      ep.StillPath,
				RuntimeMinutes: int64(ep.RuntimeMinutes),
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
				if ep.AirDate == nil || *ep.AirDate == "" || *ep.AirDate > today {
					monitored = true
				}

			case "missing":
				// For a new series, no files exist — monitor all aired episodes.
				if ep.AirDate != nil && *ep.AirDate != "" && *ep.AirDate <= today {
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

	offset := int64((page - 1) * perPage)
	limit := int64(perPage)

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
		sr, err := rowToSeries(row)
		if err != nil {
			return ListResult{}, err
		}
		series = append(series, sr)
	}

	// Populate episode + file counts for the whole page in two queries total
	// (one batched count) rather than the two-per-series that buildSeries
	// would issue — important under the single SQLite connection.
	if len(series) > 0 {
		ids := make([]string, len(series))
		for i := range series {
			ids[i] = series[i].ID
		}
		counts, err := s.q.ListEpisodeCountsBySeriesIDs(ctx, ids)
		if err != nil {
			return ListResult{}, fmt.Errorf("list episode counts: %w", err)
		}
		totals := make(map[string]int64, len(counts))
		withFile := make(map[string]int64, len(counts))
		for _, c := range counts {
			totals[c.SeriesID] = c.TotalCount
			withFile[c.SeriesID] = c.FileCount
		}
		for i := range series {
			series[i].EpisodeCount = totals[series[i].ID]
			series[i].EpisodeFileCount = withFile[series[i].ID]
		}
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
		p := req.Path
		pathVal = &p
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

// GetEpisode returns a single episode by its UUID. Returns ErrNotFound
// when no row exists. Used by the episode-detail page route.
func (s *Service) GetEpisode(ctx context.Context, episodeID string) (Episode, error) {
	row, err := s.q.GetEpisode(ctx, episodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Episode{}, ErrNotFound
		}
		return Episode{}, fmt.Errorf("get episode: %w", err)
	}
	return rowToEpisode(row), nil
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
	if len(row.AlternateTitles) > 0 && row.AlternateTitles != "null" {
		if err := json.Unmarshal([]byte(row.AlternateTitles), &alternateTitles); err != nil {
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
	if row.MetadataRefreshedAt != nil && *row.MetadataRefreshedAt != "" {
		t, err := time.Parse(time.RFC3339, *row.MetadataRefreshedAt)
		if err == nil {
			metaRefreshed = &t
		}
	}

	var runtimeMinutes int
	if row.RuntimeMinutes != nil {
		runtimeMinutes = int(*row.RuntimeMinutes)
	}

	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}

	return Series{
		ID:                  row.ID,
		TMDBID:              int(row.TmdbID),
		IMDBID:              deref(row.ImdbID),
		TVDBID:              tvdbID(row.TvdbID),
		Title:               row.Title,
		SortTitle:           row.SortTitle,
		Year:                int(row.Year),
		Overview:            row.Overview,
		RuntimeMinutes:      runtimeMinutes,
		Genres:              genres,
		PosterURL:           deref(row.PosterUrl),
		FanartURL:           deref(row.FanartUrl),
		Status:              row.Status,
		SeriesType:          row.SeriesType,
		MonitorType:         row.MonitorType,
		Network:             deref(row.Network),
		AirTime:             deref(row.AirTime),
		Certification:       deref(row.Certification),
		Monitored:           row.Monitored,
		LibraryID:           row.LibraryID,
		QualityProfileID:    row.QualityProfileID,
		Path:                deref(row.Path),
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
	if row.AbsoluteNumber != nil {
		n := int(*row.AbsoluteNumber)
		absNum = &n
	}
	airDate := ""
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
		Monitored:      row.Monitored,
		HasFile:        row.HasFile,
		StillPath:      row.StillPath,
		RuntimeMinutes: int(row.RuntimeMinutes),
	}
}

func sortTitle(title string) string {
	lower := strings.ToLower(title)
	for _, prefix := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, prefix) {
			return strings.ToLower(title[len(prefix):])
		}
	}
	return lower
}

// toNullString returns a *string; nil for an empty input. Kept under the old
// name because many call sites still use it; the underlying nullable type is
// now *string after the SQLite/sqlc migration.
func tvdbID(p *int64) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

func ptrInt64(v int) *int64 {
	n := int64(v)
	return &n
}

func toNullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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
