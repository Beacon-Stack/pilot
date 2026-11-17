package importer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

const (
	// MaxImportAttempts caps automatic retries per grab. With the backoff
	// below, 10 attempts span roughly two days before Pilot gives up.
	MaxImportAttempts = 10

	retryBaseDelay = 15 * time.Minute
	retryMaxDelay  = 6 * time.Hour

	// retryBatchLimit bounds how many failed grabs one run looks at.
	retryBatchLimit = 100
)

// retryDelay is how long to wait after the attempts-th failed attempt
// before trying again: 15m, 30m, 1h, 2h, 4h, then 6h.
func retryDelay(attempts int64) time.Duration {
	if attempts < 1 {
		return 0
	}
	d := retryBaseDelay
	for i := int64(1); i < attempts; i++ {
		d *= 2
		if d >= retryMaxDelay {
			return retryMaxDelay
		}
	}
	return d
}

// RetryResult summarises one RetryFailedImports run.
type RetryResult struct {
	Attempted int // imports re-run
	Imported  int // re-runs that succeeded
	Held      int // skipped because the library root is not writable
	NotDue    int // skipped because the backoff hasn't elapsed
	GaveUp    int // failed for the last time (hit MaxImportAttempts)
}

// RetryFailedImports re-runs the import for completed downloads whose
// previous import failed, with exponential backoff per grab.
//
// Before retrying, it checks that the target library root is writable.
// If not (the classic "library dir owned by root, container runs as uid
// 1000" setup mistake), the grab is held without spending an attempt: no
// number of retries can succeed until an operator fixes the folder, and
// once they do, the very next run imports everything.
func (s *Service) RetryFailedImports(ctx context.Context, now time.Time) (RetryResult, error) {
	var res RetryResult

	grabs, err := s.q.ListFailedImports(ctx, db.ListFailedImportsParams{
		MaxAttempts: MaxImportAttempts,
		Limit:       retryBatchLimit,
	})
	if err != nil {
		return res, fmt.Errorf("listing failed imports: %w", err)
	}
	if len(grabs) == 0 {
		return res, nil
	}

	mm, err := s.mm.Get(ctx)
	if err != nil {
		return res, fmt.Errorf("loading media management settings: %w", err)
	}

	// Per-run cache: library root -> writability error (nil = writable).
	rootCheck := map[string]error{}

	for _, g := range grabs {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}

		if last, pErr := time.Parse(time.RFC3339, g.ImportLastAttemptAt); pErr == nil &&
			now.Before(last.Add(retryDelay(g.ImportAttempts))) {
			res.NotDue++
			continue
		}

		// Only renaming imports write into the library root; with renaming
		// off the file stays where the download client put it.
		if mm.RenameEpisodes {
			root, rErr := s.libraryRootForSeries(ctx, g.SeriesID)
			if rErr != nil {
				s.logger.Warn("import retry: cannot resolve library root; skipping",
					"grab_id", g.ID, "series_id", g.SeriesID, "error", rErr)
				continue
			}
			wErr, checked := rootCheck[root]
			if !checked {
				wErr = checkWritable(root)
				rootCheck[root] = wErr
				if wErr != nil {
					s.logger.Warn("import retry: library folder is not writable; holding failed imports until it is fixed",
						"library_root", root,
						"uid", os.Getuid(),
						"gid", os.Getgid(),
						"error", wErr,
					)
				}
			}
			if wErr != nil {
				res.Held++
				continue
			}
		}

		res.Attempted++
		s.logger.Info("import retry: retrying failed import",
			"grab_id", g.ID,
			"release", g.ReleaseTitle,
			"attempt", g.ImportAttempts+1,
		)
		iErr := s.ImportFile(ctx, g.ID, g.ContentPath)
		s.recordImportOutcome(ctx, g.ID, g.ContentPath, iErr)
		if iErr != nil {
			if g.ImportAttempts+1 >= MaxImportAttempts || errors.Is(iErr, ErrUnimportableContent) {
				res.GaveUp++
				s.giveUp(ctx, g, iErr)
			} else {
				s.logger.Warn("import retry: still failing",
					"grab_id", g.ID,
					"attempt", g.ImportAttempts+1,
					"next_retry_in", retryDelay(g.ImportAttempts+1).String(),
					"error", iErr,
				)
			}
			continue
		}
		res.Imported++
	}
	return res, nil
}

// giveUp logs and publishes a final TypeImportFailed so the activity log
// shows that automatic retries are exhausted.
func (s *Service) giveUp(ctx context.Context, g db.GrabHistory, err error) {
	s.logger.Error("import retry: giving up",
		"grab_id", g.ID,
		"release", g.ReleaseTitle,
		"attempts", MaxImportAttempts,
		"error", err,
	)
	if s.bus == nil {
		return
	}
	data := map[string]any{
		"grab_id": g.ID,
		"error":   err.Error(),
		"reason":  fmt.Sprintf("gave up after %d import attempts: %s", MaxImportAttempts, err),
	}
	if series, sErr := s.q.GetSeries(ctx, g.SeriesID); sErr == nil {
		data["series_title"] = series.Title
	}
	s.bus.Publish(ctx, events.Event{
		Type:   events.TypeImportFailed,
		ShowID: g.SeriesID,
		Data:   data,
	})
}

func (s *Service) libraryRootForSeries(ctx context.Context, seriesID string) (string, error) {
	series, err := s.q.GetSeries(ctx, seriesID)
	if err != nil {
		return "", fmt.Errorf("loading series: %w", err)
	}
	lib, err := s.q.GetLibrary(ctx, series.LibraryID)
	if err != nil {
		return "", fmt.Errorf("loading library: %w", err)
	}
	return lib.RootPath, nil
}

// checkWritable reports whether the current process can create files in
// dir, by actually creating (and removing) one. Permission bits alone
// can't answer this reliably (ACLs, read-only mounts, root-squash NFS).
func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".pilot-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}
