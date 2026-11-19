package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/beacon-stack/pilot/internal/core/blocklist"
	"github.com/beacon-stack/pilot/internal/core/downloader"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/scheduler"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// Seed-target policy for cleaning up finished downloads. A torrent is
// removed from the client (with its data) once it has seeded to the
// ratio target OR has been imported for the max age — whichever comes
// first. The library copy is a separate inode (hardlink or full copy),
// so deleting the client's data never touches the library.
const (
	cleanupSeedRatioTarget = 1.0
	cleanupMaxSeedAge      = 48 * time.Hour
	cleanupBatchLimit      = 200
)

// DownloadCleanup returns a Job that removes finished torrents from the
// download client once their import outcome is known:
//   - imported + seed target met → remove with data, keep the grab
//     'completed' (only the client artifact is gone)
//   - failed import + blocklisted → the payload is known garbage
//     (fake .exe/.scr uploads); remove with data immediately and mark
//     the grab removed
//
// Without this, /downloads grew forever: nothing ever told Haul a
// download had served its purpose.
func DownloadCleanup(q db.Querier, dlSvc *downloader.Service, blocklistSvc *blocklist.Service, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "download_cleanup",
		Interval: 15 * time.Minute,
		Fn: func(ctx context.Context) {
			logger.Info("task started", "task", "download_cleanup")
			start := time.Now()
			removed, err := runDownloadCleanup(ctx, q, dlSvc, blocklistSvc, logger)
			if err != nil {
				logger.Warn("task failed",
					"task", "download_cleanup",
					"error", err,
					"duration_ms", time.Since(start).Milliseconds(),
				)
				return
			}
			logger.Info("task finished",
				"task", "download_cleanup",
				"removed", removed,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		},
	}
}

func runDownloadCleanup(ctx context.Context, q db.Querier, dlSvc *downloader.Service, blocklistSvc *blocklist.Service, logger *slog.Logger) (int, error) {
	grabs, err := q.ListCleanupCandidateGrabs(ctx, cleanupBatchLimit)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, g := range grabs {
		if g.DownloadClientID == nil || g.ClientItemID == nil {
			continue
		}

		switch g.ImportStatus {
		case "imported":
			if !seedTargetMet(ctx, dlSvc, g) {
				continue
			}
			if !removeFromClient(ctx, dlSvc, g, logger) {
				continue
			}
			if err := q.ClearGrabClientItem(ctx, g.ID); err != nil {
				logger.Warn("download_cleanup: failed to detach grab", "grab_id", g.ID, "error", err)
				continue
			}
			logger.Info("download_cleanup: removed seeded torrent after import",
				"release", g.ReleaseTitle)
			removed++

		case "failed":
			// Only delete payloads we KNOW are garbage — i.e. the import
			// failure was hard enough that the importer blocklisted the
			// release. A transiently-failed import keeps its data so a
			// manual reimport can still succeed.
			infoHash := ""
			if g.InfoHash != nil {
				infoHash = *g.InfoHash
			}
			blocked, bErr := blocklistSvc.IsBlocklistedGUIDOrInfoHash(ctx, g.ReleaseGuid, infoHash)
			if bErr != nil || !blocked {
				continue
			}
			if !removeFromClient(ctx, dlSvc, g, logger) {
				continue
			}
			if err := q.MarkGrabRemoved(ctx, g.ID); err != nil {
				logger.Warn("download_cleanup: failed to mark grab removed", "grab_id", g.ID, "error", err)
				continue
			}
			logger.Info("download_cleanup: deleted blocklisted garbage payload",
				"release", g.ReleaseTitle)
			removed++
		}
	}
	return removed, nil
}

// seedTargetMet reports whether the grab's torrent has seeded enough to
// delete: ratio target reached, or imported long enough ago. A client
// lookup failure defers the decision to a later run — EXCEPT
// ErrItemNotFound, which means the item is already gone and cleanup
// should just detach the grab (treated as target met; removeFromClient
// tolerates the missing item).
func seedTargetMet(ctx context.Context, dlSvc *downloader.Service, g db.GrabHistory) bool {
	if importedAt, err := time.Parse(time.RFC3339, g.ImportedAt); err == nil {
		if time.Since(importedAt) >= cleanupMaxSeedAge {
			return true
		}
	}
	client, err := dlSvc.ClientFor(ctx, *g.DownloadClientID)
	if err != nil {
		return false
	}
	item, err := client.Status(ctx, *g.ClientItemID)
	if err != nil {
		return errors.Is(err, plugin.ErrItemNotFound)
	}
	return item.SeedRatio >= cleanupSeedRatioTarget
}

// removeFromClient removes the item (with data) from the download
// client. An already-gone item counts as success.
func removeFromClient(ctx context.Context, dlSvc *downloader.Service, g db.GrabHistory, logger *slog.Logger) bool {
	client, err := dlSvc.ClientFor(ctx, *g.DownloadClientID)
	if err != nil {
		logger.Warn("download_cleanup: client unavailable", "grab_id", g.ID, "error", err)
		return false
	}
	if err := client.Remove(ctx, *g.ClientItemID, true); err != nil && !errors.Is(err, plugin.ErrItemNotFound) {
		logger.Warn("download_cleanup: remove failed", "grab_id", g.ID, "release", g.ReleaseTitle, "error", err)
		return false
	}
	return true
}
