package show

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
)

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

	var runtimeMinutes *int64
	if detail.RuntimeMinutes > 0 {
		v := int64(detail.RuntimeMinutes)
		runtimeMinutes = &v
	}

	now := time.Now().UTC().Format(time.RFC3339)
	updated, err := s.q.UpdateSeriesMetadata(ctx, db.UpdateSeriesMetadataParams{
		ImdbID:              keepIMDB(row.ImdbID, detail.IMDBID),
		Title:               detail.Title,
		SortTitle:           sortTitle(detail.Title),
		Year:                int64(detail.Year),
		Overview:            detail.Overview,
		RuntimeMinutes:      runtimeMinutes,
		GenresJson:          string(genresJSON),
		PosterUrl:           toNullString(detail.PosterPath),
		FanartUrl:           toNullString(detail.BackdropPath),
		Status:              detail.Status,
		Network:             toNullString(detail.Network),
		AirTime:             row.AirTime,
		Certification:       row.Certification,
		MetadataRefreshedAt: &now,
		UpdatedAt:           now,
		AlternateTitles:     string(altTitlesJSON),
		TvdbID:              keepTVDB(row.TvdbID, detail.TVDBID),
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
			p, changed := refreshedEpisode(ep, d)
			if !changed {
				continue
			}
			_, _ = s.q.UpdateEpisode(ctx, p)
		}
	}
	return nil
}

// sortTitle returns a normalised sort key by stripping common leading articles.
// refreshedEpisode merges fresh TMDB data into a stored episode. Titles,
// overviews and air dates are taken from TMDB when it has them: an
// episode added before it aired is stored as "Episode 1" with no
// overview, and without this it kept that placeholder forever, which
// then ended up in renamed file names. Empty TMDB fields never blank
// out stored ones. changed is false when there is nothing to write.
func refreshedEpisode(ep db.Episode, d tmdbtv.EpisodeDetail) (db.UpdateEpisodeParams, bool) {
	p := db.UpdateEpisodeParams{
		ID:             ep.ID,
		Title:          ep.Title,
		Overview:       ep.Overview,
		AirDate:        ep.AirDate,
		HasFile:        ep.HasFile,
		StillPath:      ep.StillPath,
		RuntimeMinutes: ep.RuntimeMinutes,
	}
	if d.Title != "" {
		p.Title = d.Title
	}
	if d.Overview != "" {
		p.Overview = d.Overview
	}
	if d.AirDate != "" {
		airDate := d.AirDate
		p.AirDate = &airDate
	}
	if d.StillPath != "" {
		p.StillPath = d.StillPath
	}
	if d.RuntimeMinutes > 0 {
		p.RuntimeMinutes = int64(d.RuntimeMinutes)
	}
	sameAirDate := (p.AirDate == nil) == (ep.AirDate == nil) &&
		(p.AirDate == nil || *p.AirDate == *ep.AirDate)
	changed := p.Title != ep.Title || p.Overview != ep.Overview || !sameAirDate ||
		p.StillPath != ep.StillPath || p.RuntimeMinutes != ep.RuntimeMinutes
	return p, changed
}

// keepTVDB prefers a fresh TVDB id from TMDB, and otherwise keeps what
// was stored (a refresh with TMDB briefly missing it shouldn't wipe it).
func keepTVDB(stored *int64, fresh int) *int64 {
	if fresh > 0 || stored == nil {
		return ptrInt64(fresh)
	}
	return stored
}

// keepIMDB is keepTVDB for the IMDb id.
func keepIMDB(stored *string, fresh string) *string {
	if fresh != "" || stored == nil {
		return &fresh
	}
	return stored
}

// ExternalIDLookup is the optional MetadataProvider method that reads a
// series' IMDb/TVDB ids on their own (tmdbtv.Client has it).
type ExternalIDLookup interface {
	GetExternalIDs(ctx context.Context, tmdbID int) (tmdbtv.ExternalIDs, error)
}

// BackfillExternalIDs looks up IMDb and TVDB ids for series that don't
// have them yet — those added before Pilot stored them. Each series is
// asked about once: a series TMDB has no id for is stored as ”/0. A
// short pause between requests keeps a large library well inside TMDB's
// rate limit. Returns how many series were updated.
func (s *Service) BackfillExternalIDs(ctx context.Context) (int, error) {
	lookup, ok := s.meta.(ExternalIDLookup)
	if !ok || s.meta == nil {
		return 0, nil
	}
	rows, err := s.q.ListSeriesMissingExternalIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list series missing external ids: %w", err)
	}
	updated := 0
	for i, row := range rows {
		if i > 0 {
			select {
			case <-ctx.Done():
				return updated, ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
		ids, err := lookup.GetExternalIDs(ctx, int(row.TmdbID))
		if err != nil {
			s.logger.Warn("external ids: lookup failed", "series_id", row.ID, "tmdb_id", row.TmdbID, "error", err)
			continue
		}
		imdb := ids.IMDBID
		if err := s.q.SetSeriesExternalIDs(ctx, db.SetSeriesExternalIDsParams{
			ImdbID: &imdb,
			TvdbID: ptrInt64(ids.TVDBID),
			ID:     row.ID,
		}); err != nil {
			return updated, fmt.Errorf("store external ids: %w", err)
		}
		updated++
	}
	if updated > 0 {
		s.logger.Info("external ids: filled in", "series", updated)
	}
	return updated, nil
}
