package v1

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/screenarr/screenarr/internal/core/blocklist"
	"github.com/screenarr/screenarr/internal/core/queue"
)

// ── Request / response shapes ────────────────────────────────────────────────

type queueItemBody struct {
	GrabID           string    `json:"id"                        doc:"Grab history UUID"`
	SeriesID         string    `json:"series_id"`
	EpisodeID        string    `json:"episode_id,omitempty"`
	ReleaseTitle     string    `json:"release_title"`
	Protocol         string    `json:"protocol"`
	Size             int64     `json:"size"`
	DownloadedBytes  int64     `json:"downloaded_bytes"`
	Status           string    `json:"status"                    doc:"queued, downloading, completed, paused, failed"`
	DownloadClientID string    `json:"download_client_id,omitempty"`
	ClientItemID     string    `json:"client_item_id,omitempty"`
	GrabbedAt        time.Time `json:"grabbed_at"`
}

type queueListOutput struct {
	Body []*queueItemBody
}

type queueItemOutput struct {
	Body *queueItemBody
}

type queueItemInput struct {
	ID string `path:"id" doc:"Grab history UUID"`
}

type queueDeleteInput struct {
	ID          string `path:"id"                           doc:"Grab history UUID"`
	DeleteFiles bool   `query:"delete_files" default:"false" doc:"Also delete downloaded data from disk"`
}

type queueDeleteOutput struct{}

type queueBlocklistInput struct {
	ID string `path:"id" doc:"Grab history UUID"`
}

type queueBlocklistOutput struct{}

// ── Helpers ──────────────────────────────────────────────────────────────────

func queueItemToBody(item queue.Item) *queueItemBody {
	return &queueItemBody{
		GrabID:           item.GrabID,
		SeriesID:         item.SeriesID,
		EpisodeID:        item.EpisodeID,
		ReleaseTitle:     item.ReleaseTitle,
		Protocol:         item.Protocol,
		Size:             item.Size,
		DownloadedBytes:  item.DownloadedBytes,
		Status:           item.Status,
		DownloadClientID: item.DownloadClientID,
		ClientItemID:     item.ClientItemID,
		GrabbedAt:        item.GrabbedAt,
	}
}

// ── Route registration ───────────────────────────────────────────────────────

// RegisterQueueRoutes registers the /api/v1/queue endpoints.
func RegisterQueueRoutes(api huma.API, svc *queue.Service, bl *blocklist.Service) {
	// GET /api/v1/queue
	huma.Register(api, huma.Operation{
		OperationID: "get-queue",
		Method:      http.MethodGet,
		Path:        "/api/v1/queue",
		Summary:     "Get the active download queue",
		Tags:        []string{"Queue"},
	}, func(ctx context.Context, _ *struct{}) (*queueListOutput, error) {
		items, err := svc.GetQueue(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get queue", err)
		}
		bodies := make([]*queueItemBody, len(items))
		for i, item := range items {
			bodies[i] = queueItemToBody(item)
		}
		return &queueListOutput{Body: bodies}, nil
	})

	// GET /api/v1/queue/{id}
	huma.Register(api, huma.Operation{
		OperationID: "get-queue-item",
		Method:      http.MethodGet,
		Path:        "/api/v1/queue/{id}",
		Summary:     "Get a single active download by grab ID",
		Tags:        []string{"Queue"},
	}, func(ctx context.Context, input *queueItemInput) (*queueItemOutput, error) {
		item, err := svc.GetQueueItem(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusNotFound, fmt.Sprintf("queue item %q not found", input.ID), err)
		}
		return &queueItemOutput{Body: queueItemToBody(item)}, nil
	})

	// DELETE /api/v1/queue/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "remove-from-queue",
		Method:        http.MethodDelete,
		Path:          "/api/v1/queue/{id}",
		Summary:       "Remove a download from the queue",
		Tags:          []string{"Queue"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *queueDeleteInput) (*queueDeleteOutput, error) {
		if err := svc.RemoveFromQueue(ctx, input.ID, input.DeleteFiles); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to remove from queue", err)
		}
		return &queueDeleteOutput{}, nil
	})

	// POST /api/v1/queue/{id}/blocklist
	// Removes the download from the client and adds the release to the blocklist
	// so it won't be grabbed again.
	huma.Register(api, huma.Operation{
		OperationID:   "blocklist-queue-item",
		Method:        http.MethodPost,
		Path:          "/api/v1/queue/{id}/blocklist",
		Summary:       "Blocklist a queued release and remove it from the download client",
		Tags:          []string{"Queue"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *queueBlocklistInput) (*queueBlocklistOutput, error) {
		// Load the item from the active queue.
		item, err := svc.GetQueueItem(ctx, input.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusNotFound, fmt.Sprintf("queue item %q not found", input.ID), err)
		}

		// Remove from download client. Ignore error so the blocklist entry is
		// still created even if the client call fails.
		_ = svc.RemoveFromQueue(ctx, input.ID, false)

		// Add to blocklist. Use a synthetic GUID (grab ID prefixed) since
		// grab_history does not store the original indexer release GUID.
		if bl != nil {
			blErr := bl.Add(ctx,
				item.SeriesID,
				item.EpisodeID,
				"grab:"+item.GrabID,
				item.ReleaseTitle,
				"",
				item.Protocol,
				item.Size,
				"blocklisted from queue",
			)
			if blErr != nil && blErr != blocklist.ErrAlreadyBlocklisted {
				return nil, huma.NewError(http.StatusInternalServerError, "failed to add to blocklist", blErr)
			}
		}

		return &queueBlocklistOutput{}, nil
	})
}
