package show

import (
	"context"
	"fmt"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

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
	for offset := int64(0); ; offset += pageSize {
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
			if e.AbsoluteNumber != nil {
				return int(*e.AbsoluteNumber), nil
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
		want := int64(counter)
		if ep.AbsoluteNumber != nil && *ep.AbsoluteNumber == want {
			continue // already correct, skip the write
		}
		if err := s.q.UpdateEpisodeAbsoluteNumber(ctx, db.UpdateEpisodeAbsoluteNumberParams{
			AbsoluteNumber: &want,
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
