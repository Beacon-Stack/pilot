package tmdbtv

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// Browse categories shared with Pilot's Discover page.
const (
	CategoryTrending   = "trending"
	CategoryPopular    = "popular"
	CategoryTopRated   = "top-rated"
	CategoryNewOnline  = "new" // new series on streaming services
	CategoryComingSoon = "coming-soon"
)

// BrowseFilter is one Discover page request: a category plus optional
// filters. Trending is a fixed TMDB list and ignores the filters.
type BrowseFilter struct {
	Category   string
	Page       int
	GenreID    int
	ProviderID int     // TMDB watch provider (e.g. 8 = Netflix); needs Region
	MinRating  float64 // TMDB vote average, 0-10
	Region     string  // ISO 3166-1 country, e.g. "US"
	Today      time.Time
}

var regionRE = regexp.MustCompile(`^[A-Z]{2}$`)

// NormalizeRegion returns a valid upper-case country code, defaulting to US.
func NormalizeRegion(r string) string {
	if len(r) == 2 {
		r = string([]byte{upper(r[0]), upper(r[1])})
	}
	if !regionRE.MatchString(r) {
		return "US"
	}
	return r
}

func upper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}

const day = 24 * time.Hour

// BrowseQuery turns a filter into the TMDB path and query parameters.
// Kept free of I/O so the date windows and filters are easy to test.
func BrowseQuery(f BrowseFilter) (string, url.Values, error) {
	p := url.Values{}
	page := f.Page
	if page < 1 {
		page = 1
	}
	p.Set("page", strconv.Itoa(page))
	region := NormalizeRegion(f.Region)
	today := f.Today
	if today.IsZero() {
		today = time.Now()
	}
	date := func(t time.Time) string { return t.UTC().Format("2006-01-02") }

	switch f.Category {
	case "", CategoryTrending:
		return "/trending/tv/week", p, nil
	case CategoryPopular:
		p.Set("sort_by", "popularity.desc")
	case CategoryTopRated:
		p.Set("sort_by", "vote_average.desc")
		p.Set("vote_count.gte", "200")
	case CategoryNewOnline:
		// New series: premiered in the last 90 days and on a streaming,
		// free or ad-supported service in this country. ("Has an episode
		// this month" filled the list with long-running network shows.)
		// A few votes keeps out spam entries, which have none.
		p.Set("sort_by", "popularity.desc")
		p.Set("first_air_date.gte", date(today.Add(-90*day)))
		p.Set("first_air_date.lte", date(today))
		p.Set("watch_region", region)
		p.Set("with_watch_monetization_types", "flatrate|free|ads")
		p.Set("vote_count.gte", "5")
	case CategoryComingSoon:
		// New series premiering in the next 90 days.
		p.Set("sort_by", "popularity.desc")
		p.Set("first_air_date.gte", date(today.Add(day)))
		p.Set("first_air_date.lte", date(today.Add(90*day)))
	default:
		return "", nil, fmt.Errorf("unknown category %q", f.Category)
	}

	if f.GenreID > 0 {
		p.Set("with_genres", strconv.Itoa(f.GenreID))
	}
	if f.ProviderID > 0 {
		// "On Netflix" means included with a subscription (or free), not
		// available to rent there.
		p.Set("with_watch_providers", strconv.Itoa(f.ProviderID))
		p.Set("watch_region", region)
		p.Set("with_watch_monetization_types", "flatrate|free|ads")
	}
	if f.MinRating > 0 {
		p.Set("vote_average.gte", strconv.FormatFloat(f.MinRating, 'f', 1, 64))
		// A 9.0 from four votes isn't a rating; top-rated sets its own floor.
		if p.Get("vote_count.gte") == "" {
			p.Set("vote_count.gte", "50")
		}
	}
	return "/discover/tv", p, nil
}

// Browse fetches one page of a Discover category with filters applied.
func (c *Client) Browse(ctx context.Context, f BrowseFilter) (*PaginatedSeries, error) {
	path, params, err := BrowseQuery(f)
	if err != nil {
		return nil, err
	}
	return c.fetchTVList(ctx, path, params)
}

// WatchProvider is a streaming service TMDB knows about in a country.
type WatchProvider struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	LogoPath string `json:"logo_path"`
}

// WatchProviders lists the streaming services available for TV in a
// country, most prominent first.
func (c *Client) WatchProviders(ctx context.Context, region string) ([]WatchProvider, error) {
	region = NormalizeRegion(region)
	var envelope struct {
		Results []struct {
			ID                int            `json:"provider_id"`
			Name              string         `json:"provider_name"`
			LogoPath          string         `json:"logo_path"`
			DisplayPriorities map[string]int `json:"display_priorities"`
		} `json:"results"`
	}
	params := url.Values{}
	params.Set("watch_region", region)
	if err := c.get(ctx, "/watch/providers/tv", params, &envelope); err != nil {
		return nil, fmt.Errorf("tmdb watch providers: %w", err)
	}
	return sortProviders(region, envelope.Results), nil
}

func sortProviders(region string, in []struct {
	ID                int            `json:"provider_id"`
	Name              string         `json:"provider_name"`
	LogoPath          string         `json:"logo_path"`
	DisplayPriorities map[string]int `json:"display_priorities"`
}) []WatchProvider {
	type ranked struct {
		WatchProvider
		prio int
	}
	rs := make([]ranked, 0, len(in))
	for _, r := range in {
		prio, ok := r.DisplayPriorities[region]
		if !ok {
			prio = 1 << 20
		}
		rs = append(rs, ranked{WatchProvider{ID: r.ID, Name: r.Name, LogoPath: r.LogoPath}, prio})
	}
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].prio != rs[j].prio {
			return rs[i].prio < rs[j].prio
		}
		return rs[i].Name < rs[j].Name
	})
	out := make([]WatchProvider, len(rs))
	for i, r := range rs {
		out[i] = r.WatchProvider
	}
	return out
}

// SearchPaginated searches TV series by title for the Discover page, with
// paging, for the Discover page.
func (c *Client) SearchPaginated(ctx context.Context, query string, page int) (*PaginatedSeries, error) {
	if page < 1 {
		page = 1
	}
	params := url.Values{}
	params.Set("query", query)
	params.Set("page", strconv.Itoa(page))
	return c.fetchTVList(ctx, "/search/tv", params)
}
