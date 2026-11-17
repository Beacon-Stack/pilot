package v1

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/beacon-stack/pilot/internal/core/backup"
)

type backupBody struct {
	Name      string `json:"name"`
	Kind      string `json:"kind" enum:"scheduled,manual"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at" doc:"RFC3339"`
}

type backupListBody struct {
	Backups []backupBody `json:"backups" doc:"Newest first"`
	Folder  string       `json:"folder"  doc:"Where backups are stored"`
}

type backupSettingsBody struct {
	IntervalDays  int `json:"interval_days"  minimum:"0" maximum:"365"  doc:"Days between scheduled backups; 0 turns them off"`
	RetentionDays int `json:"retention_days" minimum:"1" maximum:"3650" doc:"Scheduled backups are kept this long; the newest is always kept"`
}

func toBackupBody(b backup.Backup) backupBody {
	return backupBody{Name: b.Name, Kind: b.Kind, Size: b.Size, CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339)}
}

func backupError(action string, err error) error {
	var verr *backup.ValidationError
	switch {
	case errors.Is(err, backup.ErrNotFound):
		return huma.Error404NotFound("backup not found")
	case errors.Is(err, backup.ErrInvalidName):
		return huma.Error400BadRequest("invalid backup name")
	case errors.As(err, &verr):
		return huma.Error422UnprocessableEntity(verr.Msg)
	default:
		return huma.NewError(http.StatusInternalServerError, "failed to "+action, err)
	}
}

type backupNameInput struct {
	Name string `path:"name"`
}

// RegisterBackupRoutes registers the backup list, create, delete,
// restore and settings endpoints. Downloads are served by
// BackupDownloadHandler (a binary response).
func RegisterBackupRoutes(api huma.API, svc *backup.Service) {
	const tag = "Backups"

	huma.Register(api, huma.Operation{
		OperationID: "list-backups",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/backups",
		Summary:     "List backups",
		Tags:        []string{tag},
	}, func(_ context.Context, _ *struct{}) (*struct{ Body backupListBody }, error) {
		all, err := svc.List()
		if err != nil {
			return nil, backupError("list backups", err)
		}
		out := backupListBody{Backups: make([]backupBody, len(all)), Folder: svc.Dir()}
		for i, b := range all {
			out.Backups[i] = toBackupBody(b)
		}
		return &struct{ Body backupListBody }{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "create-backup",
		Method:        http.MethodPost,
		Path:          "/api/v1/system/backups",
		Summary:       "Back up the database now",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body backupBody }, error) {
		b, err := svc.Create(ctx, backup.KindManual)
		if err != nil {
			return nil, backupError("create the backup", err)
		}
		return &struct{ Body backupBody }{Body: toBackupBody(b)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-backup",
		Method:        http.MethodDelete,
		Path:          "/api/v1/system/backups/{name}",
		Summary:       "Delete a backup",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, func(_ context.Context, input *backupNameInput) (*struct{}, error) {
		if err := svc.Delete(input.Name); err != nil {
			return nil, backupError("delete the backup", err)
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "restore-backup",
		Method:      http.MethodPost,
		Path:        "/api/v1/system/backups/{name}/restore",
		Summary:     "Restore a backup on the next restart",
		Tags:        []string{tag},
	}, func(_ context.Context, input *backupNameInput) (*struct {
		Body struct {
			Message string `json:"message"`
		}
	}, error) {
		if err := svc.Restore(input.Name); err != nil {
			return nil, backupError("stage the restore", err)
		}
		out := &struct {
			Body struct {
				Message string `json:"message"`
			}
		}{}
		out.Body.Message = "Restore staged. Restart Pilot to complete the restore."
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-backup-settings",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/backup-settings",
		Summary:     "Get the backup schedule",
		Tags:        []string{tag},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body backupSettingsBody }, error) {
		st, err := svc.GetSettings(ctx)
		if err != nil {
			return nil, backupError("load backup settings", err)
		}
		return &struct{ Body backupSettingsBody }{Body: backupSettingsBody(st)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-backup-settings",
		Method:      http.MethodPut,
		Path:        "/api/v1/system/backup-settings",
		Summary:     "Update the backup schedule",
		Tags:        []string{tag},
	}, func(ctx context.Context, input *struct{ Body backupSettingsBody }) (*struct{ Body backupSettingsBody }, error) {
		if err := svc.SaveSettings(ctx, backup.Settings(input.Body)); err != nil {
			return nil, backupError("save backup settings", err)
		}
		return &struct{ Body backupSettingsBody }{Body: input.Body}, nil
	})
}

// BackupDownloadHandler serves one backup file as a download.
func BackupDownloadHandler(svc *backup.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		p, err := svc.Path(name)
		switch {
		case errors.Is(err, backup.ErrInvalidName):
			http.Error(w, `{"status":400,"title":"invalid backup name"}`, http.StatusBadRequest)
			return
		case err != nil:
			http.Error(w, `{"status":404,"title":"backup not found"}`, http.StatusNotFound)
			return
		}
		f, err := os.Open(p)
		if err != nil {
			http.Error(w, `{"status":500,"title":"failed to open backup"}`, http.StatusInternalServerError)
			logger.Warn("backup download: open", "error", err)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			http.Error(w, `{"status":500,"title":"failed to read backup"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
		http.ServeContent(w, r, name, info.ModTime(), f)
	}
}
