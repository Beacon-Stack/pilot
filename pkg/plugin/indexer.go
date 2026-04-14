package plugin

import "context"

// Capabilities describes what an indexer supports.
type Capabilities struct {
	SearchAvailable   bool
	TVSearchAvailable bool
	MovieSearch       bool
	Categories        []int // Newznab/Torznab category IDs
}

// SearchQuery is the input to an indexer search.
type SearchQuery struct {
	Query   string // free-text query, e.g. "Breaking Bad S01E05"
	TVDBID  int    // TVDB ID when available
	TMDBID  int    // TMDB ID when available
	IMDBID  string // e.g. "tt0903747"
	Year    int
	Season  int // season number for TV episode filtering
	Episode int // episode number for TV episode filtering
}

// Indexer is the plugin interface for release indexers.
type Indexer interface {
	// Name returns the human-readable plugin name, e.g. "Torznab".
	Name() string

	// Protocol returns the release download mechanism this indexer provides.
	Protocol() Protocol

	// Capabilities returns what search types this indexer supports.
	Capabilities(ctx context.Context) (Capabilities, error)

	// Search queries the indexer for releases matching the query.
	Search(ctx context.Context, q SearchQuery) ([]Release, error)

	// GetRecent returns the most recent releases from the indexer's RSS feed.
	GetRecent(ctx context.Context) ([]Release, error)

	// Test validates that the indexer is reachable and configured correctly.
	Test(ctx context.Context) error
}
