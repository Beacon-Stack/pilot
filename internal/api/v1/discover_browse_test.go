package v1

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
)

// browseMeta records the browse filter; everything else panics through
// the nil embedded interface.
type browseMeta struct {
	show.MetadataProvider
	got   tmdbtv.BrowseFilter
	query string
}

func (m *browseMeta) Browse(_ context.Context, f tmdbtv.BrowseFilter) (*tmdbtv.PaginatedSeries, error) {
	m.got = f
	return &tmdbtv.PaginatedSeries{Page: f.Page, TotalPages: 4, Results: []tmdbtv.SearchResult{{ID: 5, Title: "New Show", Year: 2026, VoteAverage: 8.1}}}, nil
}
func (m *browseMeta) WatchProviders(context.Context, string) ([]tmdbtv.WatchProvider, error) {
	return []tmdbtv.WatchProvider{{ID: 8, Name: "Netflix", LogoPath: "/n.jpg"}}, nil
}
func (m *browseMeta) SearchPaginated(_ context.Context, q string, page int) (*tmdbtv.PaginatedSeries, error) {
	m.query = q
	return &tmdbtv.PaginatedSeries{Page: page, TotalPages: 1, Results: []tmdbtv.SearchResult{{ID: 7, Title: "Found"}}}, nil
}

func TestDiscoverBrowse(t *testing.T) {
	_, api := humatest.New(t)
	meta := &browseMeta{}
	RegisterDiscoverRoutes(api, show.NewService(nil, meta, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))))

	resp := api.Get("/api/v1/discover/browse?category=new&page=2&genre=18&provider=8&min_rating=7.5&region=gb")
	if resp.Code != http.StatusOK {
		t.Fatalf("browse %d: %s", resp.Code, resp.Body.String())
	}
	f := meta.got
	if f.Category != "new" || f.Page != 2 || f.GenreID != 18 || f.ProviderID != 8 || f.MinRating != 7.5 || f.Region != "gb" || f.Today.IsZero() {
		t.Errorf("filter %+v", f)
	}
	var body discoverListBody
	_ = json.Unmarshal(resp.Body.Bytes(), &body)
	if len(body.Results) != 1 || body.Results[0].Rating != 8.1 || body.TotalPages != 4 {
		t.Errorf("body %+v", body)
	}

	if resp := api.Get("/api/v1/discover/browse?category=on-the-air"); resp.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown category: %d", resp.Code)
	}

	resp = api.Get("/api/v1/discover/providers?region=US")
	var ps []providerBody
	_ = json.Unmarshal(resp.Body.Bytes(), &ps)
	if resp.Code != http.StatusOK || len(ps) != 1 || ps[0].Name != "Netflix" {
		t.Errorf("providers %d %+v", resp.Code, ps)
	}

	resp = api.Get("/api/v1/discover/search?q=%20found%20")
	if resp.Code != http.StatusOK || meta.query != "found" {
		t.Errorf("search %d query %q", resp.Code, meta.query)
	}
	meta.query = ""
	if resp := api.Get("/api/v1/discover/search?q="); resp.Code != http.StatusOK || meta.query != "" {
		t.Errorf("empty search should not call TMDB: %d %q", resp.Code, meta.query)
	}
}

type vandalMeta struct{ browseMeta }

func (m *vandalMeta) Browse(context.Context, tmdbtv.BrowseFilter) (*tmdbtv.PaginatedSeries, error) {
	return &tmdbtv.PaginatedSeries{Page: 1, TotalPages: 1, Results: []tmdbtv.SearchResult{
		{ID: 291507, Title: "Ticket+1-888-805-5591 or US +1-888-805-5591", OriginalTitle: "The Body"},
		{ID: 2, Title: "Call 1-888-805-5591", OriginalTitle: "Call 1-888-805-5591"},
		{ID: 3, Title: "VisionQuest", OriginalTitle: "VisionQuest"},
	}}, nil
}

// A vandalised TMDB title shows the original title; junk in both is dropped.
func TestDiscoverBrowse_VandalisedTitles(t *testing.T) {
	_, api := humatest.New(t)
	RegisterDiscoverRoutes(api, show.NewService(nil, &vandalMeta{}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))))
	resp := api.Get("/api/v1/discover/browse?category=new")
	var body discoverListBody
	_ = json.Unmarshal(resp.Body.Bytes(), &body)
	if len(body.Results) != 2 || body.Results[0].Title != "The Body" || body.Results[1].Title != "VisionQuest" {
		t.Errorf("results %+v", body.Results)
	}
}
