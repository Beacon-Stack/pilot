-- name: CreateGrabHistory :one
INSERT INTO grab_history (
    id, series_id, episode_id, season_number, indexer_id,
    release_guid, release_title, release_source, release_resolution,
    release_codec, release_hdr, protocol, size,
    download_client_id, client_item_id, download_status,
    score_breakdown, grabbed_at, source, info_hash
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13,
    $14, $15, $16,
    $17, $18, $19, $20
)
RETURNING *;

-- name: ListGrabHistoryBySeries :many
SELECT * FROM grab_history WHERE series_id = $1 ORDER BY grabbed_at DESC;

-- name: ListGrabHistoryByEpisode :many
SELECT * FROM grab_history WHERE episode_id = $1 ORDER BY grabbed_at DESC;

-- name: ListGrabHistory :many
SELECT * FROM grab_history ORDER BY grabbed_at DESC LIMIT $1 OFFSET $2;

-- name: GetGrabByID :one
SELECT * FROM grab_history WHERE id = $1;

-- name: UpdateGrabStatus :exec
UPDATE grab_history SET download_status = $1, downloaded_bytes = $2 WHERE id = $3;

-- name: UpdateGrabDownloadClient :exec
UPDATE grab_history SET download_client_id = $1, client_item_id = $2 WHERE id = $3;

-- name: UpdateGrabInfoHash :exec
UPDATE grab_history SET info_hash = $1 WHERE id = $2;

-- name: ListActiveGrabs :many
SELECT * FROM grab_history WHERE download_status NOT IN ('completed', 'failed', 'removed', 'stalled') ORDER BY grabbed_at DESC;

-- name: GetGrabByClientItemID :one
SELECT * FROM grab_history WHERE client_item_id = $1 LIMIT 1;

-- name: GetGrabByInfoHash :one
SELECT * FROM grab_history WHERE info_hash = $1 ORDER BY grabbed_at DESC LIMIT 1;

-- name: MarkGrabRemoved :exec
UPDATE grab_history SET download_status = 'removed' WHERE id = $1;
