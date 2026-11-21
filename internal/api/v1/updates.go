package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/updatecheck"
	"github.com/beacon-stack/pilot/internal/version"
)

type updateCheckOutput struct {
	Body updatecheck.UpdateStatus
}

type releasesInput struct {
	Limit int `query:"limit" default:"10" minimum:"1" maximum:"30" doc:"How many recent releases"`
}

type releasesOutput struct {
	Body struct {
		Releases []updatecheck.Release `json:"releases"`
	}
}

// updateChecker is shared by every request so its short cache spares
// GitHub's 60-calls-an-hour limit for unauthenticated clients.
var updateChecker = &updatecheck.Checker{
	Repo:      "beacon-stack/pilot",
	UserAgent: "Pilot/" + version.Version,
	Client:    &http.Client{Timeout: 10 * time.Second},
}

// RegisterUpdateRoutes registers GET /api/v1/system/updates.
func RegisterUpdateRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "check-for-updates",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/updates",
		Summary:     "Check for updates",
		Description: "Compares the running build with the newest image published from beacon-stack/pilot " +
			"(the latest successful CI build on main), and lists the commits it is missing. " +
			"A release-tagged build is compared with the latest GitHub release instead.",
		Tags: []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*updateCheckOutput, error) {
		res, err := updateChecker.Check(ctx, version.Version)
		if err != nil {
			return nil, huma.NewError(http.StatusBadGateway, err.Error())
		}
		return &updateCheckOutput{Body: res}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-releases",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/releases",
		Summary:     "Recent releases and their notes",
		Description: "The newest published releases of beacon-stack/pilot, newest first, each with its release notes (markdown).",
		Tags:        []string{"System"},
	}, func(ctx context.Context, in *releasesInput) (*releasesOutput, error) {
		rels, err := updateChecker.Releases(ctx, in.Limit)
		if err != nil {
			return nil, huma.NewError(http.StatusBadGateway, err.Error())
		}
		out := &releasesOutput{}
		out.Body.Releases = rels
		if out.Body.Releases == nil {
			out.Body.Releases = []updatecheck.Release{}
		}
		return out, nil
	})
}
