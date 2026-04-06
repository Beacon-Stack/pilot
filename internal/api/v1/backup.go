package v1

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// sqliteMagic is the 16-byte header present in every SQLite database file.
var sqliteMagic = []byte("SQLite format 3\000")

// BackupHandler returns an http.HandlerFunc that streams a consistent
// SQLite backup as a downloadable file using VACUUM INTO.
func BackupHandler(db *sql.DB, dbPath string, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir := filepath.Dir(dbPath)
		tmp, err := os.CreateTemp(dir, "pilot-backup-*.db")
		if err != nil {
			http.Error(w, `{"status":500,"title":"Failed to create temp file"}`, http.StatusInternalServerError)
			logger.Warn("backup: create temp file", "error", err)
			return
		}
		tmpPath := tmp.Name()
		_ = tmp.Close()
		_ = os.Remove(tmpPath) // VACUUM INTO needs the path not to exist
		defer os.Remove(tmpPath)

		if _, err := db.Exec(fmt.Sprintf(`VACUUM INTO '%s'`, tmpPath)); err != nil {
			http.Error(w, `{"status":500,"title":"VACUUM INTO failed"}`, http.StatusInternalServerError)
			logger.Warn("backup: VACUUM INTO", "error", err)
			return
		}

		f, err := os.Open(tmpPath)
		if err != nil {
			http.Error(w, `{"status":500,"title":"Failed to open backup"}`, http.StatusInternalServerError)
			logger.Warn("backup: open temp", "error", err)
			return
		}
		defer f.Close()

		info, _ := f.Stat()
		filename := fmt.Sprintf("pilot-backup-%s.db", time.Now().Format("2006-01-02"))

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		if info != nil {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
		}

		if _, err := io.Copy(w, f); err != nil {
			logger.Warn("backup: streaming response", "error", err)
		}
	}
}

// RestoreHandler returns an http.HandlerFunc that accepts a raw SQLite
// database upload and writes it to a staging path. The actual swap happens
// on next application startup.
func RestoreHandler(dbPath string, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow up to 500 MiB for the restore upload.
		r.Body = http.MaxBytesReader(w, r.Body, 500<<20)

		stagingPath := dbPath + ".restore"
		f, err := os.Create(stagingPath)
		if err != nil {
			http.Error(w, `{"status":500,"title":"Failed to create staging file"}`, http.StatusInternalServerError)
			logger.Warn("restore: create staging", "error", err)
			return
		}

		if _, err := io.Copy(f, r.Body); err != nil {
			_ = f.Close()
			_ = os.Remove(stagingPath)
			http.Error(w, `{"status":400,"title":"Upload failed"}`, http.StatusBadRequest)
			logger.Warn("restore: copy body", "error", err)
			return
		}
		_ = f.Close()

		if err := validateSQLiteMagic(stagingPath); err != nil {
			_ = os.Remove(stagingPath)
			http.Error(w, fmt.Sprintf(`{"status":400,"title":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"Restore file saved. Restart Pilot to complete the restore."}`))
	}
}

// validateSQLiteMagic checks that the file at path starts with the SQLite
// magic header bytes.
func validateSQLiteMagic(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	header := make([]byte, 16)
	n, err := io.ReadFull(f, header)
	if err != nil || n < 16 {
		return fmt.Errorf("file too small to be a valid SQLite database")
	}

	for i := range sqliteMagic {
		if header[i] != sqliteMagic[i] {
			return fmt.Errorf("not a valid SQLite database file")
		}
	}
	return nil
}
