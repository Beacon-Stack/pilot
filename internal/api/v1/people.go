package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/show"
)

// Response body shapes

type personCreditBody struct {
	TMDBID          int    `json:"tmdb_id"                     doc:"TMDB series ID"`
	Title           string `json:"title"                       doc:"Series title"`
	Year            int    `json:"year,omitempty"              doc:"First air year"`
	PosterPath      string `json:"poster_path,omitempty"       doc:"TMDB poster path"`
	InLibrary       bool   `json:"in_library"                  doc:"Whether the series is in the library"`
	LibrarySeriesID string `json:"library_series_id,omitempty" doc:"Library series UUID, when in_library"`
}

type personDetailBody struct {
	ID                 int                `json:"id"                   doc:"TMDB person ID"`
	Name               string             `json:"name"                 doc:"Person name"`
	ProfilePath        string             `json:"profile_path,omitempty" doc:"TMDB profile image path"`
	KnownForDepartment string             `json:"known_for_department,omitempty" doc:"Primary department (Acting, Directing, etc.)"`
	Credits            []personCreditBody `json:"credits"              doc:"TV credits sorted by year descending"`
}

type personInput struct {
	ID int `path:"id" doc:"TMDB person ID"`
}

type personDetailOutput struct {
	Body *personDetailBody
}

// Route registration

// RegisterPeopleRoutes registers the /api/v1/people/{id} endpoint.
func RegisterPeopleRoutes(api huma.API, showSvc *show.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "get-person",
		Method:      http.MethodGet,
		Path:        "/api/v1/people/{id}",
		Summary:     "Get person details and TV credits",
		Description: "Returns person info and their TV credits from TMDB, cross-referenced with the library.",
		Tags:        []string{"People"},
	}, func(ctx context.Context, input *personInput) (*personDetailOutput, error) {
		person, credits, err := showSvc.GetPerson(ctx, input.ID)
		if err != nil {
			return nil, mapShowError(err, "person")
		}

		bodies := make([]personCreditBody, 0, len(credits))
		for _, c := range credits {
			bodies = append(bodies, personCreditBody{
				TMDBID:          c.TMDBID,
				Title:           c.Title,
				Year:            c.Year,
				PosterPath:      c.PosterPath,
				InLibrary:       c.InLibrary,
				LibrarySeriesID: c.LibrarySeriesID,
			})
		}

		return &personDetailOutput{Body: &personDetailBody{
			ID:                 person.ID,
			Name:               person.Name,
			ProfilePath:        person.ProfilePath,
			KnownForDepartment: person.KnownForDepartment,
			Credits:            bodies,
		}}, nil
	})
}
