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
	VoteAverage   float64
}

// PaginatedSeries is a page of series results from a TMDB list endpoint
// (popular, trending, browse, discover-by-genre).
type PaginatedSeries struct {
	Results    []SearchResult
	Page       int
	TotalPages int
}

// Genre is a TMDB TV genre.
type Genre struct {
	ID   int
	Name string
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
	// IMDBID ("tt...") and TVDBID come from external_ids; empty/zero when
	// TMDB has none.
	IMDBID string
	TVDBID int

	// The fields below are only populated by GetSeriesExtended (which
	// requests append_to_response=credits,recommendations). GetSeries
	// leaves them empty to keep the Add/RefreshMetadata payload light.
	Rating          float64
	Cast            []CastMember
	Crew            []CrewMember
	Recommendations []SeriesRecommendation
}

// CastMember is a top-billed cast entry from the credits append.
type CastMember struct {
	ID          int
	Name        string
	Character   string
	ProfilePath string
	Order       int
}

// CrewMember is a key crew entry (creator, EP, writer, director) from
// the credits append.
type CrewMember struct {
	ID          int
	Name        string
	Job         string
	Department  string
	ProfilePath string
}

// SeriesRecommendation is a recommended series from the recommendations
// append.
type SeriesRecommendation struct {
	TMDBID     int
	Title      string
	Year       int
	PosterPath string
}

// Person holds basic TMDB person info from /person/{id}.
type Person struct {
	ID                 int
	Name               string
	ProfilePath        string
	KnownForDepartment string
}

// PersonCredit is a single TV series a person is credited on, from
// /person/{id}/tv_credits.
type PersonCredit struct {
	TMDBID     int
	Title      string
	Year       int
	PosterPath string
	Order      int
	// EpisodeCount is how many episodes the person appears in / worked on.
	// Used to rank real roles above one-off talk-show / self appearances,
	// since the cast `order` field is unreliable for TV.
	EpisodeCount int
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

// ExternalIDs are a series' ids on other databases, as TMDB knows them.
type ExternalIDs struct {
	IMDBID string // "tt0903747"; empty when unknown
	TVDBID int    // 0 when unknown
}
