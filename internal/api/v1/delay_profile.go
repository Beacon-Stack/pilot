package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/delay"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

type delayProfileBody struct {
	UsenetEnabled          bool   `json:"usenet_enabled"`
	TorrentEnabled         bool   `json:"torrent_enabled"`
	PreferredProtocol      string `json:"preferred_protocol"        enum:"usenet,torrent"`
	UsenetDelayMinutes     int    `json:"usenet_delay_minutes"      minimum:"0" doc:"Wait this long after a usenet release is published"`
	TorrentDelayMinutes    int    `json:"torrent_delay_minutes"     minimum:"0" doc:"Wait this long after a torrent release is published"`
	BypassIfHighestQuality bool   `json:"bypass_if_highest_quality" doc:"Grab immediately when the release is the best quality the profile allows"`
}

type pendingReleaseBody struct {
	ID            string `json:"id"`
	SeriesID      string `json:"series_id"`
	SeriesTitle   string `json:"series_title"`
	EpisodeID     string `json:"episode_id"`
	SeasonNumber  int64  `json:"season_number"`
	EpisodeNumber int64  `json:"episode_number"`
	ReleaseTitle  string `json:"release_title"`
	Protocol      string `json:"protocol"`
	Quality       string `json:"quality"`
	EligibleAt    string `json:"eligible_at" doc:"When the release may be grabbed (RFC3339)"`
	AddedAt       string `json:"added_at"`
}

// RegisterDelayRoutes registers the delay profile and pending-release
// endpoints.
func RegisterDelayRoutes(api huma.API, q db.Querier) {
	store := delay.NewStore(q)

	huma.Register(api, huma.Operation{
		OperationID: "get-delay-profile",
		Method:      http.MethodGet,
		Path:        "/api/v1/delay-profile",
		Summary:     "Get the delay profile",
		Tags:        []string{"Delay Profile"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body delayProfileBody }, error) {
		p, err := store.Get(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to load the delay profile", err)
		}
		return &struct{ Body delayProfileBody }{Body: delayProfileBody(p)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-delay-profile",
		Method:      http.MethodPut,
		Path:        "/api/v1/delay-profile",
		Summary:     "Update the delay profile",
		Tags:        []string{"Delay Profile"},
	}, func(ctx context.Context, input *struct{ Body delayProfileBody }) (*struct{ Body delayProfileBody }, error) {
		p := delay.Profile(input.Body)
		if err := store.Set(ctx, p); err != nil {
			var verr *delay.ValidationError
			if errors.As(err, &verr) {
				return nil, huma.Error422UnprocessableEntity(verr.Msg)
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to save the delay profile", err)
		}
		return &struct{ Body delayProfileBody }{Body: input.Body}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-pending-releases",
		Method:      http.MethodGet,
		Path:        "/api/v1/queue/pending",
		Summary:     "Releases held back by the delay profile",
		Tags:        []string{"Queue"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []pendingReleaseBody }, error) {
		rows, err := q.ListPendingReleasesWithSeries(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list pending releases", err)
		}
		out := make([]pendingReleaseBody, 0, len(rows))
		for _, r := range rows {
			out = append(out, pendingReleaseBody{
				ID:            r.ID,
				SeriesID:      r.SeriesID,
				SeriesTitle:   r.SeriesTitle,
				EpisodeID:     r.EpisodeID,
				SeasonNumber:  r.SeasonNumber,
				EpisodeNumber: r.EpisodeNumber,
				ReleaseTitle:  r.ReleaseTitle,
				Protocol:      r.Protocol,
				Quality:       r.QualityName,
				EligibleAt:    r.EligibleAt,
				AddedAt:       r.AddedAt,
			})
		}
		return &struct{ Body []pendingReleaseBody }{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-pending-release",
		Method:        http.MethodDelete,
		Path:          "/api/v1/queue/pending/{id}",
		Summary:       "Drop a pending release",
		Description:   "The release is forgotten. If it shows up in RSS again it will be considered again.",
		Tags:          []string{"Queue"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*struct{}, error) {
		if err := q.DeletePendingRelease(ctx, input.ID); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to drop the pending release", err)
		}
		return nil, nil
	})
}
