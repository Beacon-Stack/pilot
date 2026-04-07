package tmdbtv

// SearchResult is a single item from a /search/tv response.
type SearchResult struct {
	ID            int
	Title         string
	OriginalTitle string
	Overview      string
	FirstAirDate  string
	Year          int
	PosterPath    string
	BackdropPath  string
	Popularity    float64
}

// SeriesDetail holds full series metadata returned by /tv/{id}.
type SeriesDetail struct {
	ID             int
	Title          string
	OriginalTitle  string
	Overview       string
	FirstAirDate   string
	Year           int
	RuntimeMinutes int
	Genres         []string
	PosterPath     string
	BackdropPath   string
	// Status is one of "continuing", "ended", or "upcoming".
	Status  string
	Network string
	Seasons []SeasonSummary
}

// SeasonSummary is a brief season entry embedded in SeriesDetail.
type SeasonSummary struct {
	SeasonNumber int
	EpisodeCount int
	AirDate      string
}

// EpisodeDetail holds per-episode metadata returned by /tv/{id}/season/{n}.
type EpisodeDetail struct {
	ID             int
	SeasonNumber   int
	EpisodeNumber  int
	Title          string
	Overview       string
	AirDate        string
	StillPath      string
	RuntimeMinutes int
}
