package show

import (
	"context"
	"fmt"
	"sort"

	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
)

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

// LookupDetail fetches full series metadata (including cast, crew, and
// recommendations) for a single TMDB ID without adding anything to the
// library. It also reports whether the series already exists in the
// library and, if so, its library series ID — so the Discover detail page
// can link straight to it instead of offering an add flow.
func (s *Service) LookupDetail(ctx context.Context, tmdbID int) (*tmdbtv.SeriesDetail, bool, string, error) {
	if s.meta == nil {
		return nil, false, "", ErrMetadataNotConfigured
	}

	detail, err := s.meta.GetSeriesExtended(ctx, tmdbID)
	if err != nil {
		return nil, false, "", fmt.Errorf("fetch series detail: %w", err)
	}

	// Library membership is best-effort: a lookup failure here shouldn't
	// fail the whole detail fetch, so treat any error as "not in library".
	var inLibrary bool
	var librarySeriesID string
	if row, err := s.q.GetSeriesByTMDBID(ctx, int64(tmdbID)); err == nil {
		inLibrary = true
		librarySeriesID = row.ID
	}

	return detail, inLibrary, librarySeriesID, nil
}

// PersonCredit is a single TV-series credit for a person, annotated with
// whether the series is in the library.
type PersonCredit struct {
	tmdbtv.PersonCredit
	InLibrary       bool
	LibrarySeriesID string
}

// GetPerson fetches a TMDB person and the TV series they are credited on,
// sorted most-recent-first and cross-referenced against the library.
func (s *Service) GetPerson(ctx context.Context, personID int) (*tmdbtv.Person, []PersonCredit, error) {
	if s.meta == nil {
		return nil, nil, ErrMetadataNotConfigured
	}

	person, err := s.meta.GetPerson(ctx, personID)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch person: %w", err)
	}

	personType := "actor"
	if person.KnownForDepartment == "Directing" {
		personType = "director"
	}

	raw, err := s.meta.GetPersonTVCredits(ctx, personID, personType)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch person tv credits: %w", err)
	}

	// Dedupe by series (a person can be both cast and crew on a show),
	// keeping the highest episode count.
	bySeries := make(map[int]tmdbtv.PersonCredit, len(raw))
	for _, c := range raw {
		if existing, ok := bySeries[c.TMDBID]; !ok || c.EpisodeCount > existing.EpisodeCount {
			bySeries[c.TMDBID] = c
		}
	}
	deduped := make([]tmdbtv.PersonCredit, 0, len(bySeries))
	for _, c := range bySeries {
		deduped = append(deduped, c)
	}

	// Rank by screen time (episode count), then most-recent-first, so the
	// shows a person is actually known for surface above one-off talk-show
	// or self appearances. Cap to a focused "known for" list.
	sort.Slice(deduped, func(i, j int) bool {
		if deduped[i].EpisodeCount != deduped[j].EpisodeCount {
			return deduped[i].EpisodeCount > deduped[j].EpisodeCount
		}
		return deduped[i].Year > deduped[j].Year
	})
	const maxCredits = 24
	if len(deduped) > maxCredits {
		deduped = deduped[:maxCredits]
	}

	credits := make([]PersonCredit, 0, len(deduped))
	for _, c := range deduped {
		credit := PersonCredit{PersonCredit: c}
		if row, lookupErr := s.q.GetSeriesByTMDBID(ctx, int64(c.TMDBID)); lookupErr == nil {
			credit.InLibrary = true
			credit.LibrarySeriesID = row.ID
		}
		credits = append(credits, credit)
	}

	return person, credits, nil
}

// DiscoverBrowse returns a page of a Discover category with filters.
func (s *Service) DiscoverBrowse(ctx context.Context, f tmdbtv.BrowseFilter) (*tmdbtv.PaginatedSeries, error) {
	if s.meta == nil {
		return nil, ErrMetadataNotConfigured
	}
	return s.meta.Browse(ctx, f)
}

// DiscoverProviders lists the streaming services in a country.
func (s *Service) DiscoverProviders(ctx context.Context, region string) ([]tmdbtv.WatchProvider, error) {
	if s.meta == nil {
		return nil, ErrMetadataNotConfigured
	}
	return s.meta.WatchProviders(ctx, region)
}

// DiscoverSearch searches series by title, a page at a time.
func (s *Service) DiscoverSearch(ctx context.Context, query string, page int) (*tmdbtv.PaginatedSeries, error) {
	if s.meta == nil {
		return nil, ErrMetadataNotConfigured
	}
	return s.meta.SearchPaginated(ctx, query, page)
}

// DiscoverByGenre returns a page of series in the given TMDB genre.
func (s *Service) DiscoverByGenre(ctx context.Context, genreID, page int) (*tmdbtv.PaginatedSeries, error) {
	if s.meta == nil {
		return nil, ErrMetadataNotConfigured
	}
	return s.meta.DiscoverTVByGenre(ctx, genreID, page)
}

// DiscoverGenres returns the list of TMDB TV genres.
func (s *Service) DiscoverGenres(ctx context.Context) ([]tmdbtv.Genre, error) {
	if s.meta == nil {
		return nil, ErrMetadataNotConfigured
	}
	return s.meta.GetTVGenres(ctx)
}
