-- name: CreateGrabHistory :one
INSERT INTO grab_history (
    id, series_id, episode_id, season_number, indexer_id,
    release_guid, release_title, release_source, release_resolution,
    release_codec, release_hdr, protocol, size,
    download_client_id, client_item_id, download_status,
    score_breakdown, grabbed_at, source, info_hash
) VALUES (
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?,
    ?, ?, ?, ?
)
RETURNING *;

-- name: ListGrabHistoryBySeries :many
SELECT * FROM grab_history WHERE series_id = ? ORDER BY grabbed_at DESC;

-- name: ListGrabHistory :many
SELECT * FROM grab_history ORDER BY grabbed_at DESC LIMIT ? OFFSET ?;

-- name: GetGrabByID :one
SELECT * FROM grab_history WHERE id = ?;

-- name: UpdateGrabStatus :exec
UPDATE grab_history SET download_status = ?, downloaded_bytes = ? WHERE id = ?;

-- name: UpdateGrabDownloadClient :exec
UPDATE grab_history SET download_client_id = ?, client_item_id = ? WHERE id = ?;

-- name: UpdateGrabInfoHash :exec
UPDATE grab_history SET info_hash = ? WHERE id = ?;

-- name: ListActiveGrabs :many
SELECT * FROM grab_history WHERE download_status NOT IN ('completed', 'failed', 'removed', 'stalled') ORDER BY grabbed_at DESC;

-- name: ListStaleActiveGrabs :many
-- Stuck-downloading grabs that the stallwatcher should expire because
-- they can't make progress. Two cohorts:
--   1. info_hash IS NULL/empty: pilot recorded the grab but never got an
--      info_hash back from the download client. The stallwatcher's
--      info_hash-keyed pipeline is blind to these - they sit in
--      `downloading` forever. We expire them by age.
--   2. info_hash IS set: stallwatcher already has a better signal (haul
--      reports the stall reason). Those are handled by the haul-stall
--      path; not returned here.
-- Returned rows are old enough that the operator's expectation is "this
-- should have completed by now or be reported as stalled."
SELECT * FROM grab_history
 WHERE download_status IN ('downloading', 'queued', 'pending')
   AND (info_hash IS NULL OR info_hash = '')
   AND grabbed_at < sqlc.arg('older_than')
 ORDER BY grabbed_at ASC;

-- name: GetGrabByInfoHash :one
SELECT * FROM grab_history WHERE info_hash = ? ORDER BY grabbed_at DESC LIMIT 1;

-- name: MarkGrabRemoved :exec
UPDATE grab_history SET download_status = 'removed' WHERE id = ?;

-- name: ListGrabHistoryByStatusSince :many
-- Used by the Activity page's "Recently imported" and "Needs attention" rails
-- to window grabs by terminal status and time. grab_history.grabbed_at is
-- TEXT-encoded RFC3339 so the comparison is lexicographic but order-preserving.
SELECT * FROM grab_history
WHERE download_status = sqlc.arg('status')
  AND grabbed_at > sqlc.arg('since')
ORDER BY grabbed_at DESC
LIMIT sqlc.arg('limit');

-- name: SetGrabImportState :exec
UPDATE grab_history SET
    import_status = ?,
    imported_at   = ?,
    import_error  = ?
WHERE id = ?;

-- name: ListCleanupCandidateGrabs :many
-- Grabs still present on a download client whose import outcome is known --
-- the download_cleanup job removes them (with data) once the seed target
-- is met (imported) or immediately (failed + blocklisted garbage).
SELECT * FROM grab_history
WHERE download_status = 'completed'
  AND import_status != ''
  AND client_item_id IS NOT NULL
ORDER BY grabbed_at ASC
LIMIT ?;

-- name: ClearGrabClientItem :exec
-- Detaches a grab from its download-client item after seed-target
-- cleanup removed the torrent. download_status stays 'completed' --
-- the grab did succeed; only the client-side artifact is gone.
UPDATE grab_history SET client_item_id = NULL WHERE id = ?;

-- name: NoteGrabImportAttempt :exec
-- Bookkeeping for automatic import retry, written alongside
-- SetGrabImportState. content_path is only overwritten with a non-empty
-- value; min_attempts lets a hard failure exhaust the retry budget.
UPDATE grab_history SET
    content_path           = COALESCE(NULLIF(CAST(sqlc.arg('content_path') AS TEXT), ''), content_path),
    import_attempts        = MAX(import_attempts + 1, CAST(sqlc.arg('min_attempts') AS INTEGER)),
    import_last_attempt_at = CAST(sqlc.arg('attempted_at') AS TEXT)
WHERE id = sqlc.arg('id');

-- name: ListFailedImports :many
-- Completed downloads whose import failed and that still have retry
-- budget. Oldest attempt first so a backlog drains fairly.
SELECT * FROM grab_history
WHERE import_status = 'failed'
  AND download_status = 'completed'
  AND content_path != ''
  AND import_attempts < sqlc.arg('max_attempts')
ORDER BY import_last_attempt_at ASC
LIMIT sqlc.arg('limit');
