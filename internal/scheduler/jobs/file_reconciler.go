package jobs

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/internal/scheduler"
)

// fileReconcilerInterval is how often the file-existence reconciler runs.
// 6h matches the QW5 design — long enough that a maintenance window of
// the storage layer doesn't trigger a flurry of false retry events,
// short enough that an operator removing files via the shell sees the
// library reflect that within a single afternoon.
const fileReconcilerInterval = 6 * time.Hour

// fileReconcilerCompletedWindow caps pass-2 lookback so the reconciler
// doesn't retry-spam ancient grabs. A grab from a year ago whose file
// is missing is almost certainly not retry-worthy — the user has long
// moved on.
const fileReconcilerCompletedWindow = 30 * 24 * time.Hour

// FileExistenceReconciler returns a Job that walks every episode_files
// row and stats the underlying path. When the file is gone (operator
// rm'd it via the shell, mount disappeared, etc.), the row is deleted
// and episodes.has_file is flipped back to FALSE. A second pass picks
// up any `completed` grab whose episode currently lacks has_file and
// fires TypeImportRetryNeeded so the activity log surfaces the
// divergence and (future) auto-retry can act on it.
//
// This is the on-ramp toward the haul-as-source-of-truth migration
// from the lifecycle-trust plan: we don't yet trust haul to drive
// state, but we do periodically reconcile what pilot believes against
// what the filesystem actually reports.
//
// The reconciler is intentionally read-then-write per row. Tearing
// through it as a single SQL DELETE … WHERE NOT EXISTS would be
// faster but couldn't fire per-episode events, and would make a
// stale-mount scenario (every stat returns ENOENT for 30 seconds)
// catastrophic instead of recoverable. The current shape limits the
// blast radius to whatever paths actually return ENOENT during this
// pass — and the next pass cleans up anything that flipped between
// runs.
func FileExistenceReconciler(q db.Querier, bus *events.Bus, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "file_existence_reconciler",
		Interval: fileReconcilerInterval,
		Fn: func(ctx context.Context) {
			logger.Info("task started", "task", "file_existence_reconciler")
			start := time.Now()
			missing, completedRetry, err := runFileExistenceReconciler(ctx, q, bus, logger, time.Now().UTC())
			if err != nil {
				logger.Warn("task failed",
					"task", "file_existence_reconciler",
					"error", err,
					"duration_ms", time.Since(start).Milliseconds(),
				)
				return
			}
			logger.Info("task finished",
				"task", "file_existence_reconciler",
				"missing_files", missing,
				"retry_needed", completedRetry,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		},
	}
}

// runFileExistenceReconciler is the testable kernel. Returns
// (filesFlippedMissing, retryEventsFired, err). The reconciler proceeds
// even when individual rows fail — one bad path must not skip the rest
// of the library.
func runFileExistenceReconciler(
	ctx context.Context,
	q db.Querier,
	bus *events.Bus,
	logger *slog.Logger,
	now time.Time,
) (int, int, error) {
	files, err := q.ListAllEpisodeFiles(ctx)
	if err != nil {
		return 0, 0, err
	}

	// Episodes whose has_file we just flipped, OR which we discover in
	// pass 2 are completed-but-not-imported. Deduped via map so each
	// episode receives at most one TypeImportRetryNeeded per run, even
	// when it appears in both passes.
	retryEpisodes := map[string]string{} // episode_id -> seriesID for the event
	missingCount := 0

	// ── Pass 1: stat every recorded file, prune the ones gone from disk ──
	for _, f := range files {
		if _, statErr := os.Stat(f.Path); statErr != nil {
			if !errors.Is(statErr, fs.ErrNotExist) {
				// Permission error, broken mount, etc — log but don't
				// flip has_file; the file may well still be there once
				// the underlying storage recovers.
				logger.Warn("file_reconciler: stat error (not flipping)",
					"episode_id", f.EpisodeID,
					"path", f.Path,
					"error", statErr,
				)
				continue
			}

			// File is definitively gone. Delete the episode_files row,
			// flip the has_file flag, queue a retry event.
			if dErr := q.DeleteEpisodeFile(ctx, f.ID); dErr != nil {
				logger.Warn("file_reconciler: delete episode_files row failed",
					"file_id", f.ID, "error", dErr)
				continue
			}
			if hErr := q.UpdateEpisodeHasFile(ctx, db.UpdateEpisodeHasFileParams{
				HasFile: false,
				ID:      f.EpisodeID,
			}); hErr != nil {
				logger.Warn("file_reconciler: flip has_file=FALSE failed",
					"episode_id", f.EpisodeID, "error", hErr)
				continue
			}
			missingCount++
			retryEpisodes[f.EpisodeID] = f.SeriesID
			logger.Warn("file_reconciler: file missing on disk; flipping has_file=FALSE",
				"episode_id", f.EpisodeID,
				"series_id", f.SeriesID,
				"path", f.Path,
			)
		}
	}

	// ── Pass 2: completed grabs whose episode currently lacks a file ──
	// Catches the "import failed silently — episode_files row never
	// got written" case (Maul S01 class). Bounded window so we don't
	// retry-spam ancient grabs.
	since := now.Add(-fileReconcilerCompletedWindow).Format(time.RFC3339)
	completed, err := q.ListGrabHistoryByStatusSince(ctx, db.ListGrabHistoryByStatusSinceParams{
		Status: "completed",
		Since:  since,
		Limit:  500,
	})
	if err != nil {
		// Pass 1 may already have made progress — log the partial
		// failure but return what we did.
		logger.Warn("file_reconciler: list completed grabs failed", "error", err)
	} else {
		for _, g := range completed {
			if g.EpisodeID == nil || *g.EpisodeID == "" {
				continue // season-pack grabs have no single episode to retry
			}
			epID := *g.EpisodeID
			if _, alreadyQueued := retryEpisodes[epID]; alreadyQueued {
				continue
			}
			ep, epErr := q.GetEpisode(ctx, epID)
			if epErr != nil {
				continue // episode deleted out from under us — fine
			}
			if !ep.HasFile {
				retryEpisodes[epID] = g.SeriesID
				logger.Warn("file_reconciler: completed grab has no episode_files row; queuing retry",
					"grab_id", g.ID,
					"episode_id", epID,
					"series_id", g.SeriesID,
					"release", g.ReleaseTitle,
				)
			}
		}
	}

	if bus != nil {
		for episodeID, seriesID := range retryEpisodes {
			bus.Publish(ctx, events.Event{
				Type:      events.TypeImportRetryNeeded,
				ShowID:    seriesID,
				EpisodeID: episodeID,
				Data: map[string]any{
					"reason": "file_missing_or_never_imported",
				},
			})
		}
	}

	return missingCount, len(retryEpisodes), nil
}
