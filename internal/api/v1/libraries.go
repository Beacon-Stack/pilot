package v1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/library"
)

// ── Request / response shapes ─────────────────────────────────────────────────

type libraryBody struct {
	ID                      string    `json:"id"                           doc:"Library UUID"`
	Name                    string    `json:"name"                         doc:"Human-readable library name"`
	RootPath                string    `json:"root_path"                    doc:"Absolute path to the library root"`
	DefaultQualityProfileID string    `json:"default_quality_profile_id"   doc:"Default quality profile UUID"`
	NamingFormat            *string   `json:"naming_format,omitempty"      doc:"Optional file naming template"`
	FolderFormat            *string   `json:"folder_format,omitempty"      doc:"Optional folder naming template"`
	MinFreeSpaceGB          int       `json:"min_free_space_gb"            doc:"Minimum free disk space in gigabytes"`
	Tags                    []string  `json:"tags"                         doc:"User-defined tags"`
	CreatedAt               time.Time `json:"created_at"                   doc:"Creation timestamp (UTC)"`
	UpdatedAt               time.Time `json:"updated_at"                   doc:"Last update timestamp (UTC)"`
}

type libraryInput struct {
	Name                    string   `json:"name"                          doc:"Human-readable library name"`
	RootPath                string   `json:"root_path"                     doc:"Absolute path to the library root"`
	DefaultQualityProfileID string   `json:"default_quality_profile_id"    doc:"Default quality profile UUID"`
	NamingFormat            *string  `json:"naming_format,omitempty"       doc:"Optional file naming template"`
	FolderFormat            *string  `json:"folder_format,omitempty"       doc:"Optional folder naming template"`
	MinFreeSpaceGB          *int     `json:"min_free_space_gb,omitempty"   doc:"Minimum free disk space in gigabytes (default: 0)"`
	Tags                    []string `json:"tags,omitempty"                doc:"User-defined tags (default: [])"`
}

// ── Output wrappers ───────────────────────────────────────────────────────────

type libraryOutput struct {
	Body *libraryBody
}

type libraryListOutput struct {
	Body []*libraryBody
}

type libraryDeleteOutput struct{}

// ── Input wrappers ────────────────────────────────────────────────────────────

type libraryCreateInput struct {
	Body libraryInput
}

type libraryGetInput struct {
	ID string `path:"id"`
}

type libraryUpdateInput struct {
	ID   string `path:"id"`
	Body libraryInput
}

type libraryDeleteInput struct {
	ID string `path:"id"`
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func libToBody(lib library.Library) *libraryBody {
	return &libraryBody{
		ID:                      lib.ID,
		Name:                    lib.Name,
		RootPath:                lib.RootPath,
		DefaultQualityProfileID: lib.DefaultQualityProfileID,
		NamingFormat:            lib.NamingFormat,
		FolderFormat:            lib.FolderFormat,
		MinFreeSpaceGB:          lib.MinFreeSpaceGB,
		Tags:                    lib.Tags,
		CreatedAt:               lib.CreatedAt,
		UpdatedAt:               lib.UpdatedAt,
	}
}

func libInputToCreateRequest(in libraryInput) library.CreateRequest {
	minFree := 0
	if in.MinFreeSpaceGB != nil {
		minFree = *in.MinFreeSpaceGB
	}
	tags := in.Tags
	if tags == nil {
		tags = []string{}
	}
	return library.CreateRequest{
		Name:                    in.Name,
		RootPath:                in.RootPath,
		DefaultQualityProfileID: in.DefaultQualityProfileID,
		NamingFormat:            in.NamingFormat,
		FolderFormat:            in.FolderFormat,
		MinFreeSpaceGB:          minFree,
		Tags:                    tags,
	}
}

// validateLibraryRootPath checks that the given path exists and is a directory.
func validateLibraryRootPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("root path %q is not accessible — if running in Docker, ensure it is mounted as a volume", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("root path %q is not a directory", path)
	}
	return nil
}

// ── Route registration ────────────────────────────────────────────────────────

// RegisterLibraryRoutes registers all /api/v1/libraries endpoints.
func RegisterLibraryRoutes(api huma.API, librarySvc *library.Service) {
	// GET /api/v1/libraries
	huma.Register(api, huma.Operation{
		OperationID: "list-libraries",
		Method:      http.MethodGet,
		Path:        "/api/v1/libraries",
		Summary:     "List libraries",
		Tags:        []string{"Libraries"},
	}, func(ctx context.Context, _ *struct{}) (*libraryListOutput, error) {
		libs, err := librarySvc.List(ctx)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to list libraries", err)
		}
		bodies := make([]*libraryBody, len(libs))
		for i, lib := range libs {
			bodies[i] = libToBody(lib)
		}
		return &libraryListOutput{Body: bodies}, nil
	})

	// POST /api/v1/libraries
	huma.Register(api, huma.Operation{
		OperationID:   "create-library",
		Method:        http.MethodPost,
		Path:          "/api/v1/libraries",
		Summary:       "Create a library",
		Tags:          []string{"Libraries"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *libraryCreateInput) (*libraryOutput, error) {
		if err := validateLibraryRootPath(input.Body.RootPath); err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, err.Error())
		}
		lib, err := librarySvc.Create(ctx, libInputToCreateRequest(input.Body))
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "failed to create library", err)
		}
		return &libraryOutput{Body: libToBody(lib)}, nil
	})

	// GET /api/v1/libraries/{id}
	huma.Register(api, huma.Operation{
		OperationID: "get-library",
		Method:      http.MethodGet,
		Path:        "/api/v1/libraries/{id}",
		Summary:     "Get a library",
		Tags:        []string{"Libraries"},
	}, func(ctx context.Context, input *libraryGetInput) (*libraryOutput, error) {
		lib, err := librarySvc.Get(ctx, input.ID)
		if err != nil {
			if errors.Is(err, library.ErrNotFound) {
				return nil, huma.Error404NotFound("library not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to get library", err)
		}
		return &libraryOutput{Body: libToBody(lib)}, nil
	})

	// PUT /api/v1/libraries/{id}
	huma.Register(api, huma.Operation{
		OperationID: "update-library",
		Method:      http.MethodPut,
		Path:        "/api/v1/libraries/{id}",
		Summary:     "Update a library",
		Tags:        []string{"Libraries"},
	}, func(ctx context.Context, input *libraryUpdateInput) (*libraryOutput, error) {
		if err := validateLibraryRootPath(input.Body.RootPath); err != nil {
			return nil, huma.NewError(http.StatusUnprocessableEntity, err.Error())
		}
		lib, err := librarySvc.Update(ctx, input.ID, libInputToCreateRequest(input.Body))
		if err != nil {
			if errors.Is(err, library.ErrNotFound) {
				return nil, huma.Error404NotFound("library not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to update library", err)
		}
		return &libraryOutput{Body: libToBody(lib)}, nil
	})

	// DELETE /api/v1/libraries/{id}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-library",
		Method:        http.MethodDelete,
		Path:          "/api/v1/libraries/{id}",
		Summary:       "Delete a library",
		Tags:          []string{"Libraries"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *libraryDeleteInput) (*libraryDeleteOutput, error) {
		if err := librarySvc.Delete(ctx, input.ID); err != nil {
			if errors.Is(err, library.ErrNotFound) {
				return nil, huma.Error404NotFound("library not found")
			}
			return nil, huma.NewError(http.StatusInternalServerError, "failed to delete library", err)
		}
		return &libraryDeleteOutput{}, nil
	})
}
