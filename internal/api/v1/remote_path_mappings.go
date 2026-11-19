package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/remotepath"
)

type remotePathMappingBody struct {
	ID               string `json:"id"`
	DownloadClientID string `json:"download_client_id" doc:"Limit to one download client; empty = any client"`
	RemotePath       string `json:"remote_path"        doc:"The folder as the download client reports it"`
	LocalPath        string `json:"local_path"         doc:"The same folder as Pilot sees it"`
}

type remotePathMappingInput struct {
	DownloadClientID string `json:"download_client_id,omitempty" doc:"Limit to one download client; empty = any client"`
	RemotePath       string `json:"remote_path"                  minLength:"1" doc:"The folder as the download client reports it, e.g. /downloads"`
	LocalPath        string `json:"local_path"                   minLength:"1" doc:"The same folder as Pilot sees it, e.g. /mnt/media/downloads"`
}

func (in remotePathMappingInput) toRequest() remotepath.Request {
	return remotepath.Request{DownloadClientID: in.DownloadClientID, RemotePath: in.RemotePath, LocalPath: in.LocalPath}
}

func remotePathMappingToBody(m remotepath.Mapping) remotePathMappingBody {
	return remotePathMappingBody{ID: m.ID, DownloadClientID: m.DownloadClientID, RemotePath: m.RemotePath, LocalPath: m.LocalPath}
}

func remotePathMappingError(action string, err error) error {
	var verr *remotepath.ValidationError
	switch {
	case errors.Is(err, remotepath.ErrNotFound):
		return huma.Error404NotFound("remote path mapping not found")
	case errors.As(err, &verr):
		return huma.Error422UnprocessableEntity(verr.Msg)
	default:
		return huma.NewError(http.StatusInternalServerError, "failed to "+action+" remote path mapping", err)
	}
}

// RegisterRemotePathMappingRoutes registers CRUD for remote path mappings.
func RegisterRemotePathMappingRoutes(api huma.API, svc *remotepath.Service) {
	const tag = "Remote Path Mappings"

	huma.Register(api, huma.Operation{
		OperationID: "list-remote-path-mappings",
		Method:      http.MethodGet,
		Path:        "/api/v1/remote-path-mappings",
		Summary:     "List remote path mappings",
		Tags:        []string{tag},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []remotePathMappingBody }, error) {
		ms, err := svc.List(ctx)
		if err != nil {
			return nil, remotePathMappingError("list", err)
		}
		out := make([]remotePathMappingBody, len(ms))
		for i, m := range ms {
			out[i] = remotePathMappingToBody(m)
		}
		return &struct{ Body []remotePathMappingBody }{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-remote-path-mapping",
		Method:        http.MethodPost,
		Path:          "/api/v1/remote-path-mappings",
		Summary:       "Create a remote path mapping",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *struct{ Body remotePathMappingInput }) (*struct{ Body remotePathMappingBody }, error) {
		m, err := svc.Create(ctx, input.Body.toRequest())
		if err != nil {
			return nil, remotePathMappingError("create", err)
		}
		return &struct{ Body remotePathMappingBody }{Body: remotePathMappingToBody(m)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-remote-path-mapping",
		Method:      http.MethodPut,
		Path:        "/api/v1/remote-path-mappings/{id}",
		Summary:     "Update a remote path mapping",
		Tags:        []string{tag},
	}, func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body remotePathMappingInput
	}) (*struct{ Body remotePathMappingBody }, error) {
		m, err := svc.Update(ctx, input.ID, input.Body.toRequest())
		if err != nil {
			return nil, remotePathMappingError("update", err)
		}
		return &struct{ Body remotePathMappingBody }{Body: remotePathMappingToBody(m)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-remote-path-mapping",
		Method:        http.MethodDelete,
		Path:          "/api/v1/remote-path-mappings/{id}",
		Summary:       "Delete a remote path mapping",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*struct{}, error) {
		if err := svc.Delete(ctx, input.ID); err != nil {
			return nil, remotePathMappingError("delete", err)
		}
		return nil, nil
	})
}
