package plugin

import "context"

// ImportListItem is a single series returned by an import list source.
type ImportListItem struct {
	TMDbID     int    // required — canonical identifier
	IMDbID     string // optional; used for display/logging
	Title      string
	Year       int
	PosterPath string // optional; TMDb poster path (e.g. "/abc123.jpg")
}

// ImportList is the plugin interface for import list sources.
// Implementations fetch series from external services (TMDb, Trakt, Plex, etc.)
// and return them as a flat list. The sync service handles deduplication,
// exclusion checks, and adding series to the library.
type ImportList interface {
	Name() string
	Fetch(ctx context.Context) ([]ImportListItem, error)
	Test(ctx context.Context) error
}

// TMDBInjectable is an optional interface for import list plugins that need
// the shared TMDB client. The service calls SetTMDBClient before Fetch()/Test().
// The parameter is typed as any to avoid an import cycle with internal/metadata/tmdb.
type TMDBInjectable interface {
	SetTMDBClient(client any)
}

// TraktInjectable is an optional interface for import list plugins that need
// the shared Trakt client. The service calls SetTraktClient before Fetch()/Test().
// The parameter is typed as any to avoid an import cycle with internal/trakt.
type TraktInjectable interface {
	SetTraktClient(client any)
}
