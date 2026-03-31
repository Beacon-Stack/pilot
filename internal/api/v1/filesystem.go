package v1

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

type fsBrowseInput struct {
	Path string `query:"path" doc:"Absolute directory path to list. Defaults to /."`
}

type fsDirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type fsBrowseBody struct {
	Path   string       `json:"path"`
	Parent *string      `json:"parent"`
	Dirs   []fsDirEntry `json:"dirs"`
}

type fsBrowseOutput struct {
	Body fsBrowseBody
}

// RegisterFilesystemRoutes registers the server-side filesystem browsing endpoint.
func RegisterFilesystemRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "fs-browse",
		Method:      http.MethodGet,
		Path:        "/api/v1/fs/browse",
		Summary:     "List subdirectories at the given path",
		Tags:        []string{"Filesystem"},
	}, func(_ context.Context, input *fsBrowseInput) (*fsBrowseOutput, error) {
		path := input.Path
		if path == "" {
			path = "/"
		}

		// Ensure the path is absolute and clean.
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) {
			return nil, huma.NewError(http.StatusBadRequest, "path must be absolute")
		}

		// Block virtual/pseudo filesystems that are never valid media paths.
		for _, blocked := range []string{"/proc", "/sys", "/dev"} {
			if path == blocked || strings.HasPrefix(path, blocked+"/") {
				return nil, huma.NewError(http.StatusForbidden, "browsing "+blocked+" is not allowed")
			}
		}

		// Resolve symlinks and re-check to prevent traversal into blocked paths.
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil && resolved != path {
			for _, blocked := range []string{"/proc", "/sys", "/dev"} {
				if resolved == blocked || strings.HasPrefix(resolved, blocked+"/") {
					return nil, huma.NewError(http.StatusForbidden, "path resolves to "+blocked+" which is not allowed")
				}
			}
			path = resolved
		}

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, huma.NewError(http.StatusBadRequest, "path does not exist")
			}
			return nil, huma.NewError(http.StatusBadRequest, "cannot access path: "+err.Error())
		}
		if !info.IsDir() {
			return nil, huma.NewError(http.StatusBadRequest, "path is not a directory")
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, "cannot read directory: "+err.Error())
		}

		dirs := make([]fsDirEntry, 0, len(entries))
		for _, e := range entries {
			// Skip hidden entries and non-directories.
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dirs = append(dirs, fsDirEntry{
				Name: e.Name(),
				Path: filepath.Join(path, e.Name()),
			})
		}

		var parent *string
		if path != "/" {
			p := filepath.Dir(path)
			parent = &p
		}

		out := &fsBrowseOutput{}
		out.Body.Path = path
		out.Body.Parent = parent
		out.Body.Dirs = dirs
		return out, nil
	})
}
