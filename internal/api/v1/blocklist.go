package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/screenarr/screenarr/internal/core/blocklist"
)

type blocklistEntryBody struct {
	ID           string    `json:"id"`
	SeriesID     string    `json:"series_id"`
	SeriesTitle  string    `json:"series_title"`
	EpisodeID    string    `json:"episode_id,omitempty"`
	ReleaseGUID  string    `json:"release_guid"`
	ReleaseTitle string    `json:"release_title"`
	IndexerID    string    `json:"indexer_id,omitempty"`
	Protocol     string    `json:"protocol"`
	Size         int64     `json:"size"`
	AddedAt      time.Time `json:"added_at"`
	Notes        string    `json:"notes,omitempty"`
}

type blocklistListInput struct {
	Page    int `query:"page"     default:"1"  minimum:"1"`
	PerPage int `query:"per_page" default:"50" minimum:"1" maximum:"200"`
}

type blocklistListOutput struct {
	Body struct {
		Items   []*blocklistEntryBody `json:"items"`
		Total   int64                 `json:"total"`
		Page    int                   `json:"page"`
		PerPage int                   `json:"per_page"`
	}
}

type blocklistDeleteInput struct {
	ID string `path:"id"`
}

// RegisterBlocklistRoutes registers the blocklist management endpoints.
func RegisterBlocklistRoutes(api huma.API, svc *blocklist.Service) {
	// GET /api/v1/blocklist
	huma.Register(api, huma.Operation{
		OperationID: "list-blocklist",
		Method:      http.MethodGet,
		Path:        "/api/v1/blocklist",
		Summary:     "List blocklisted releases",
		Tags:        []string{"Blocklist"},
	}, func(ctx context.Context, input *blocklistListInput) (*blocklistListOutput, error) {
		entries, total, err := svc.List(ctx, input.Page, input.PerPage)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list blocklist", err)
		}
		items := make([]*blocklistEntryBody, len(entries))
		for i, e := range entries {
			items[i] = &blocklistEntryBody{
				ID:           e.ID,
				SeriesID:     e.SeriesID,
				SeriesTitle:  e.SeriesTitle,
				EpisodeID:    e.EpisodeID,
				ReleaseGUID:  e.ReleaseGUID,
				ReleaseTitle: e.ReleaseTitle,
				IndexerID:    e.IndexerID,
				Protocol:     e.Protocol,
				Size:         e.Size,
				AddedAt:      e.AddedAt,
				Notes:        e.Notes,
			}
		}
		out := &blocklistListOutput{}
		out.Body.Items = items
		out.Body.Total = total
		out.Body.Page = input.Page
		out.Body.PerPage = input.PerPage
		return out, nil
	})

	// DELETE /api/v1/blocklist/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-blocklist-entry",
		Method:        http.MethodDelete,
		Path:          "/api/v1/blocklist/{id}",
		Summary:       "Delete a single blocklist entry",
		Tags:          []string{"Blocklist"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *blocklistDeleteInput) (*struct{}, error) {
		if err := svc.Delete(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete blocklist entry", err)
		}
		return nil, nil
	})

	// DELETE /api/v1/blocklist
	huma.Register(api, huma.Operation{
		OperationID:   "clear-blocklist",
		Method:        http.MethodDelete,
		Path:          "/api/v1/blocklist",
		Summary:       "Clear the entire blocklist",
		Tags:          []string{"Blocklist"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		if err := svc.Clear(ctx); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to clear blocklist", err)
		}
		return nil, nil
	})
}
