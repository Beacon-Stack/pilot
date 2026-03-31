-- name: CreateGrabHistory :one
INSERT INTO grab_history (
    id, series_id, episode_id, season_number, indexer_id,
    release_guid, release_title, release_source, release_resolution,
    release_codec, release_hdr, protocol, size,
    download_client_id, client_item_id, download_status,
    score_breakdown, grabbed_at
) VALUES (
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?,
    ?, ?
)
RETURNING *;

-- name: ListGrabHistoryBySeries :many
SELECT * FROM grab_history WHERE series_id = ? ORDER BY grabbed_at DESC;

-- name: ListGrabHistoryByEpisode :many
SELECT * FROM grab_history WHERE episode_id = ? ORDER BY grabbed_at DESC;

-- name: ListGrabHistory :many
SELECT * FROM grab_history ORDER BY grabbed_at DESC LIMIT ? OFFSET ?;

-- name: GetGrabByID :one
SELECT * FROM grab_history WHERE id = ?;

-- name: UpdateGrabStatus :exec
UPDATE grab_history SET download_status = ?, downloaded_bytes = ? WHERE id = ?;

-- name: UpdateGrabDownloadClient :exec
UPDATE grab_history SET download_client_id = ?, client_item_id = ? WHERE id = ?;

-- name: ListActiveGrabs :many
SELECT * FROM grab_history WHERE download_status NOT IN ('completed', 'failed', 'removed') ORDER BY grabbed_at DESC;

-- name: GetGrabByClientItemID :one
SELECT * FROM grab_history WHERE client_item_id = ? LIMIT 1;

-- name: MarkGrabRemoved :exec
UPDATE grab_history SET download_status = 'removed' WHERE id = ?;
