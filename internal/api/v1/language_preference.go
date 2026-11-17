package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/language"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

type languagePreferenceBody struct {
	Languages []string `json:"languages" doc:"Wanted audio languages. Empty means any language. Untagged releases count as English; MULTI releases match any language."`
}

type languagePreferenceOutput struct {
	Languages []string `json:"languages"`
	Available []string `json:"available" doc:"Languages that can be chosen"`
}

func languagePreferenceResponse(p language.Preference) *struct{ Body languagePreferenceOutput } {
	avail := make([]string, 0, len(language.Known))
	for _, l := range language.Known {
		avail = append(avail, string(l))
	}
	langs := p.Languages
	if langs == nil {
		langs = []string{}
	}
	return &struct{ Body languagePreferenceOutput }{Body: languagePreferenceOutput{Languages: langs, Available: avail}}
}

// RegisterLanguageRoutes registers the language preference endpoints.
func RegisterLanguageRoutes(api huma.API, q db.Querier) {
	store := language.NewStore(q)

	huma.Register(api, huma.Operation{
		OperationID: "get-language-preference",
		Method:      http.MethodGet,
		Path:        "/api/v1/language-preference",
		Summary:     "Get the language preference",
		Tags:        []string{"Language"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body languagePreferenceOutput }, error) {
		p, err := store.Get(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to load the language preference", err)
		}
		return languagePreferenceResponse(p), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-language-preference",
		Method:      http.MethodPut,
		Path:        "/api/v1/language-preference",
		Summary:     "Update the language preference",
		Tags:        []string{"Language"},
	}, func(ctx context.Context, input *struct{ Body languagePreferenceBody }) (*struct{ Body languagePreferenceOutput }, error) {
		p := language.Preference{Languages: input.Body.Languages}
		if err := store.Set(ctx, p); err != nil {
			var verr *language.ValidationError
			if errors.As(err, &verr) {
				return nil, huma.Error422UnprocessableEntity(verr.Msg)
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to save the language preference", err)
		}
		return languagePreferenceResponse(p), nil
	})
}
