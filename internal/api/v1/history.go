package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	dbsqlite "github.com/beacon-media/pilot/internal/db/generated/sqlite"
)

type historyItemBody struct {
	ID                string          `json:"id"`
	SeriesID          string          `json:"series_id"`
	EpisodeID         *string         `json:"episode_id,omitempty"`
	SeasonNumber      *int64          `json:"season_number,omitempty"`
	ReleaseTitle      string          `json:"release_title"`
	ReleaseSource     string          `json:"release_source,omitempty"`
	ReleaseResolution string          `json:"release_resolution,omitempty"`
	Protocol          string          `json:"protocol"`
	Size              int64           `json:"size"`
	DownloadStatus    string          `json:"download_status"`
	GrabbedAt         time.Time       `json:"grabbed_at"`
	ScoreBreakdown    json.RawMessage `json:"score_breakdown,omitempty"`
}

type historyListInput struct {
	Limit          int    `query:"limit"           default:"100" minimum:"1" maximum:"1000"`
	DownloadStatus string `query:"download_status" doc:"Filter by status: completed, failed, queued, downloading, paused, removed"`
	Page           int    `query:"page"            default:"1" minimum:"1"`
}

type historyListOutput struct {
	Body []*historyItemBody
}

// RegisterHistoryRoutes registers the global grab history endpoint and the
// per-series grab history endpoint.
func RegisterHistoryRoutes(humaAPI huma.API, q dbsqlite.Querier) {
	huma.Register(humaAPI, huma.Operation{
		OperationID: "list-history",
		Method:      http.MethodGet,
		Path:        "/api/v1/history",
		Summary:     "List grab history",
		Tags:        []string{"History"},
	}, func(ctx context.Context, input *historyListInput) (*historyListOutput, error) {
		limit := int64(input.Limit)
		if limit == 0 {
			limit = 100
		}
		page := input.Page
		if page < 1 {
			page = 1
		}
		offset := int64((page - 1)) * limit

		rows, err := q.ListGrabHistory(ctx, dbsqlite.ListGrabHistoryParams{
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list history", err)
		}

		items := toHistoryItems(rows, input.DownloadStatus)
		return &historyListOutput{Body: items}, nil
	})

	type seriesHistoryInput struct {
		ID string `path:"id"`
	}

	huma.Register(humaAPI, huma.Operation{
		OperationID: "list-series-history",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/history",
		Summary:     "List grab history for a specific series",
		Tags:        []string{"History"},
	}, func(ctx context.Context, input *seriesHistoryInput) (*historyListOutput, error) {
		rows, err := q.ListGrabHistoryBySeries(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list series history", err)
		}
		items := toHistoryItems(rows, "")
		return &historyListOutput{Body: items}, nil
	})
}

func toHistoryItems(rows []dbsqlite.GrabHistory, statusFilter string) []*historyItemBody {
	items := make([]*historyItemBody, 0, len(rows))
	for _, r := range rows {
		if statusFilter != "" && r.DownloadStatus != statusFilter {
			continue
		}
		grabbedAt, _ := time.Parse(time.RFC3339, r.GrabbedAt)
		item := &historyItemBody{
			ID:                r.ID,
			SeriesID:          r.SeriesID,
			EpisodeID:         r.EpisodeID,
			SeasonNumber:      r.SeasonNumber,
			ReleaseTitle:      r.ReleaseTitle,
			ReleaseSource:     r.ReleaseSource,
			ReleaseResolution: r.ReleaseResolution,
			Protocol:          r.Protocol,
			Size:              r.Size,
			DownloadStatus:    r.DownloadStatus,
			GrabbedAt:         grabbedAt,
		}
		if r.ScoreBreakdown != nil && *r.ScoreBreakdown != "" {
			item.ScoreBreakdown = json.RawMessage(*r.ScoreBreakdown)
		}
		items = append(items, item)
	}
	return items
}
