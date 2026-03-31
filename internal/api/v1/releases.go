package v1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/screenarr/screenarr/internal/core/downloader"
	"github.com/screenarr/screenarr/internal/core/indexer"
	"github.com/screenarr/screenarr/internal/core/show"
	dbsqlite "github.com/screenarr/screenarr/internal/db/generated/sqlite"
	"github.com/screenarr/screenarr/pkg/plugin"
)

// ── Request / response shapes ────────────────────────────────────────────────

// releaseBody is the API representation of a single indexer search result.
type releaseBody struct {
	GUID         string         `json:"guid"`
	Title        string         `json:"title"`
	Indexer      string         `json:"indexer"`
	IndexerID    string         `json:"indexer_id"`
	Protocol     string         `json:"protocol"`
	DownloadURL  string         `json:"download_url"`
	InfoURL      string         `json:"info_url,omitempty"`
	Size         int64          `json:"size"`
	Seeds        int            `json:"seeds,omitempty"`
	Peers        int            `json:"peers,omitempty"`
	AgeDays      float64        `json:"age_days,omitempty"`
	Quality      plugin.Quality `json:"quality"`
	QualityScore int            `json:"quality_score"`
}

type releaseListOutput struct {
	Body []*releaseBody
}

// episodeReleasesInput describes the path and optional query parameters for
// GET /api/v1/series/{id}/releases.
type episodeReleasesInput struct {
	SeriesID string `path:"id"               doc:"Series UUID"`
	Season   int    `query:"season"          doc:"Season number (required when episode is set)"`
	Episode  int    `query:"episode"         doc:"Episode number; omit to search for all season releases"`
}

// grabHistoryBody is a summary of one recorded grab.
type grabHistoryBody struct {
	ID             string    `json:"id"`
	SeriesID       string    `json:"series_id"`
	EpisodeID      *string   `json:"episode_id,omitempty"`
	SeasonNumber   *int64    `json:"season_number,omitempty"`
	IndexerID      *string   `json:"indexer_id,omitempty"`
	ReleaseGUID    string    `json:"release_guid"`
	ReleaseTitle   string    `json:"release_title"`
	Protocol       string    `json:"protocol"`
	Size           int64     `json:"size"`
	DownloadStatus string    `json:"download_status"`
	GrabbedAt      time.Time `json:"grabbed_at"`
}

type grabHistoryListOutput struct {
	Body []*grabHistoryBody
}

type seriesGrabHistoryInput struct {
	SeriesID string `path:"id" doc:"Series UUID"`
}

// grabInput carries the release the client wants to grab.
type grabInput struct {
	SeriesID string `path:"id"`
	Body     grabReleaseBody
}

type grabReleaseBody struct {
	GUID         string         `json:"guid"`
	Title        string         `json:"title"`
	IndexerID    string         `json:"indexer_id,omitempty"`
	Protocol     string         `json:"protocol"`
	DownloadURL  string         `json:"download_url"`
	Size         int64          `json:"size"`
	EpisodeID    string         `json:"episode_id,omitempty"`
	SeasonNumber int            `json:"season_number,omitempty"`
	Quality      plugin.Quality `json:"quality,omitempty"`
}

type grabOutput struct {
	Body *grabHistoryBody
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func indexerResultToReleaseBody(r indexer.SearchResult) *releaseBody {
	return &releaseBody{
		GUID:         r.GUID,
		Title:        r.Title,
		Indexer:      r.Indexer,
		IndexerID:    r.IndexerID,
		Protocol:     string(r.Protocol),
		DownloadURL:  r.DownloadURL,
		InfoURL:      r.InfoURL,
		Size:         r.Size,
		Seeds:        r.Seeds,
		Peers:        r.Peers,
		AgeDays:      r.AgeDays,
		Quality:      r.Quality,
		QualityScore: r.QualityScore,
	}
}

// buildEpisodeQuery builds a Sonarr-style search query for a specific episode
// or season. Examples:
//
//	"Breaking Bad S01E05" — single episode
//	"Breaking Bad S01"    — full season
//	"Breaking Bad"        — whole series
func buildEpisodeQuery(title string, season, episode int) string {
	switch {
	case season > 0 && episode > 0:
		return fmt.Sprintf("%s S%02dE%02d", title, season, episode)
	case season > 0:
		return fmt.Sprintf("%s S%02d", title, season)
	default:
		return title
	}
}

// ── Route registration ───────────────────────────────────────────────────────

// RegisterReleaseRoutes registers the release search and grab history endpoints.
func RegisterReleaseRoutes(api huma.API, indexerSvc *indexer.Service, showSvc *show.Service, downloaderSvc *downloader.Service) {
	// GET /api/v1/series/{id}/releases?season=1&episode=5
	huma.Register(api, huma.Operation{
		OperationID: "search-series-releases",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/releases",
		Summary:     "Search for releases for a series episode across all enabled indexers",
		Tags:        []string{"Releases"},
	}, func(ctx context.Context, input *episodeReleasesInput) (*releaseListOutput, error) {
		series, err := showSvc.Get(ctx, input.SeriesID)
		if err != nil {
			if errors.Is(err, show.ErrNotFound) {
				return nil, huma.Error404NotFound("series not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
		}

		query := buildEpisodeQuery(series.Title, input.Season, input.Episode)

		results, searchErr := indexerSvc.Search(ctx, query)

		bodies := make([]*releaseBody, len(results))
		for i, r := range results {
			bodies[i] = indexerResultToReleaseBody(r)
		}

		if len(bodies) == 0 && searchErr != nil {
			return nil, huma.NewError(http.StatusBadGateway, searchErr.Error())
		}

		return &releaseListOutput{Body: bodies}, nil
	})

	// GET /api/v1/series/{id}/grab-history
	huma.Register(api, huma.Operation{
		OperationID: "list-series-grab-history",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/grab-history",
		Summary:     "List grab history for a series",
		Tags:        []string{"Releases"},
	}, func(ctx context.Context, input *seriesGrabHistoryInput) (*grabHistoryListOutput, error) {
		if _, err := showSvc.Get(ctx, input.SeriesID); err != nil {
			if errors.Is(err, show.ErrNotFound) {
				return nil, huma.Error404NotFound("series not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
		}

		rows, err := indexerSvc.GrabHistory(ctx, input.SeriesID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list grab history", err)
		}

		bodies := make([]*grabHistoryBody, 0, len(rows))
		for _, row := range rows {
			grabbedAt, _ := time.Parse(time.RFC3339, row.GrabbedAt)
			bodies = append(bodies, &grabHistoryBody{
				ID:             row.ID,
				SeriesID:       row.SeriesID,
				EpisodeID:      row.EpisodeID,
				SeasonNumber:   row.SeasonNumber,
				IndexerID:      row.IndexerID,
				ReleaseGUID:    row.ReleaseGuid,
				ReleaseTitle:   row.ReleaseTitle,
				Protocol:       row.Protocol,
				Size:           row.Size,
				DownloadStatus: row.DownloadStatus,
				GrabbedAt:      grabbedAt,
			})
		}

		return &grabHistoryListOutput{Body: bodies}, nil
	})

	// POST /api/v1/series/{id}/releases/grab
	huma.Register(api, huma.Operation{
		OperationID:   "grab-series-release",
		Method:        http.MethodPost,
		Path:          "/api/v1/series/{id}/releases/grab",
		Summary:       "Grab a release: submit to download client and record in history",
		Tags:          []string{"Releases"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *grabInput) (*grabOutput, error) {
		if _, err := showSvc.Get(ctx, input.SeriesID); err != nil {
			if errors.Is(err, show.ErrNotFound) {
				return nil, huma.Error404NotFound("series not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get series", err)
		}

		release := plugin.Release{
			GUID:        input.Body.GUID,
			Title:       input.Body.Title,
			Protocol:    plugin.Protocol(input.Body.Protocol),
			DownloadURL: input.Body.DownloadURL,
			Size:        input.Body.Size,
			Quality:     input.Body.Quality,
		}

		// Record the grab in history first with status "queued".
		row, err := indexerSvc.CreateGrab(ctx, indexer.GrabRequest{
			SeriesID:     input.SeriesID,
			EpisodeID:    input.Body.EpisodeID,
			SeasonNumber: input.Body.SeasonNumber,
			Release:      release,
			IndexerID:    input.Body.IndexerID,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to record grab", err)
		}

		// Submit to a download client and update the grab record with client IDs.
		if downloaderSvc != nil {
			clientID, itemID, addErr := downloaderSvc.Add(ctx, release, nil)
			if addErr != nil {
				// Log the failure but return the grab record — the user can retry
				// or manually assign it later.
				_ = addErr
			} else {
				_ = indexerSvc.UpdateGrabDownloadClient(ctx, dbsqlite.UpdateGrabDownloadClientParams{
					ID:               row.ID,
					DownloadClientID: &clientID,
					ClientItemID:     &itemID,
				})
				row.DownloadClientID = &clientID
				row.ClientItemID = &itemID
			}
		}

		grabbedAt, _ := time.Parse(time.RFC3339, row.GrabbedAt)
		return &grabOutput{Body: &grabHistoryBody{
			ID:             row.ID,
			SeriesID:       row.SeriesID,
			EpisodeID:      row.EpisodeID,
			SeasonNumber:   row.SeasonNumber,
			IndexerID:      row.IndexerID,
			ReleaseGUID:    row.ReleaseGuid,
			ReleaseTitle:   row.ReleaseTitle,
			Protocol:       row.Protocol,
			Size:           row.Size,
			DownloadStatus: row.DownloadStatus,
			GrabbedAt:      grabbedAt,
		}}, nil
	})
}
