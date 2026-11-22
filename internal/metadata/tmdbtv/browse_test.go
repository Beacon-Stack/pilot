package tmdbtv

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

var today = time.Date(2026, 11, 22, 15, 0, 0, 0, time.UTC)

func TestBrowseQuery_TVCategories(t *testing.T) {
	cases := []struct {
		cat, path string
		want      map[string]string
	}{
		{"trending", "/trending/tv/week", map[string]string{"page": "1"}},
		{"popular", "/discover/tv", map[string]string{"sort_by": "popularity.desc"}},
		{"top-rated", "/discover/tv", map[string]string{"sort_by": "vote_average.desc", "vote_count.gte": "200"}},
		{"new", "/discover/tv", map[string]string{
			"air_date.gte": "2026-10-23", "air_date.lte": "2026-11-22",
			"watch_region": "US", "with_watch_monetization_types": "flatrate|free|ads",
		}},
		{"coming-soon", "/discover/tv", map[string]string{"first_air_date.gte": "2026-11-23", "first_air_date.lte": "2027-02-20"}},
	}
	for _, c := range cases {
		path, p, err := BrowseQuery(BrowseFilter{Category: c.cat, Today: today})
		if err != nil {
			t.Fatalf("%s: %v", c.cat, err)
		}
		if path != c.path {
			t.Errorf("%s: path %s, want %s", c.cat, path, c.path)
		}
		for k, v := range c.want {
			if p.Get(k) != v {
				t.Errorf("%s: %s=%q, want %q", c.cat, k, p.Get(k), v)
			}
		}
	}
}

func TestBrowseQuery_TVFilters(t *testing.T) {
	_, p, err := BrowseQuery(BrowseFilter{Category: "popular", GenreID: 18, ProviderID: 337, MinRating: 7.5, Region: "ca", Today: today})
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"with_genres": "18", "with_watch_providers": "337", "watch_region": "CA", "vote_average.gte": "7.5", "vote_count.gte": "50"} {
		if p.Get(k) != v {
			t.Errorf("%s=%q, want %q", k, p.Get(k), v)
		}
	}
	if _, _, err := BrowseQuery(BrowseFilter{Category: "on-the-air"}); err == nil {
		t.Error("unknown category should error")
	}
}

func TestBrowse_TV(t *testing.T) {
	var got url.Values
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/discover/tv":
			got = r.URL.Query()
			_, _ = w.Write([]byte(`{"page":1,"total_pages":3,"results":[{"id":5,"name":"New Show","first_air_date":"2026-11-02","vote_average":8.1}]}`))
		case "/watch/providers/tv":
			_, _ = w.Write([]byte(`{"results":[{"provider_id":2,"provider_name":"B","display_priorities":{"US":5}},{"provider_id":1,"provider_name":"A","display_priorities":{"US":1}}]}`))
		case "/search/tv":
			_, _ = w.Write([]byte(`{"page":1,"total_pages":1,"results":[{"id":7,"name":"Found"}]}`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	pr, err := c.Browse(context.Background(), BrowseFilter{Category: "new", Today: today})
	if err != nil || len(pr.Results) != 1 || pr.Results[0].Title != "New Show" || pr.Results[0].VoteAverage != 8.1 || got.Get("air_date.lte") != "2026-11-22" {
		t.Errorf("browse %+v %v %v", pr, err, got)
	}
	ps, err := c.WatchProviders(context.Background(), "US")
	if err != nil || len(ps) != 2 || ps[0].Name != "A" {
		t.Errorf("providers %+v %v", ps, err)
	}
	sr, err := c.SearchPaginated(context.Background(), "found", 1)
	if err != nil || len(sr.Results) != 1 || sr.Results[0].Title != "Found" {
		t.Errorf("search %+v %v", sr, err)
	}
}
