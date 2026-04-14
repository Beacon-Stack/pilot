-- name: CreateBlocklistEntry :one
INSERT INTO blocklist (id, series_id, episode_id, release_guid, release_title, indexer_id,
    protocol, size, added_at, notes, reason, info_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: IsBlocklisted :one
SELECT COUNT(*) FROM blocklist WHERE release_guid = $1;

-- name: IsBlocklistedByTitle :one
SELECT COUNT(*) FROM blocklist WHERE release_title = $1;

-- name: IsBlocklistedByGuidOrInfoHash :one
-- Two-keyed dedup: a release can be on the blocklist under either its
-- original indexer GUID OR its content info hash (which is stable across
-- indexers). The stall watcher populates info_hash when Haul has observed
-- the torrent, so different-GUID-same-content cases are caught.
SELECT COUNT(*) FROM blocklist
WHERE release_guid = $1 OR (info_hash IS NOT NULL AND info_hash = $2);

-- name: CountRecentStallsForEpisode :one
-- Circuit breaker for auto-re-search: how many stall-reason blocklist
-- entries exist for this (series, episode) within the last 24 hours?
-- Auto-re-search caps at 3 to avoid infinite retry loops when every
-- release for an episode happens to be dead. Pass episode_id as a
-- sql.NullString; null matches blocklist rows where episode_id IS NULL.
SELECT COUNT(*) FROM blocklist
WHERE series_id = $1
  AND episode_id IS NOT DISTINCT FROM $2
  AND reason LIKE 'stall_%'
  AND added_at > NOW() - INTERVAL '24 hours';

-- name: ListBlocklist :many
SELECT b.*, s.title AS series_title
FROM blocklist b JOIN series s ON s.id = b.series_id
ORDER BY b.added_at DESC
LIMIT $1 OFFSET $2;

-- name: CountBlocklist :one
SELECT COUNT(*) FROM blocklist;

-- name: DeleteBlocklistEntry :exec
DELETE FROM blocklist WHERE id = $1;

-- name: DeleteBlocklistEntryByGUID :exec
DELETE FROM blocklist WHERE release_guid = $1;

-- name: ClearBlocklist :exec
DELETE FROM blocklist;
