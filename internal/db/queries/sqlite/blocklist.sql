-- name: CreateBlocklistEntry :one
INSERT INTO blocklist (id, series_id, episode_id, release_guid, release_title, indexer_id,
    protocol, size, added_at, notes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: IsBlocklisted :one
SELECT COUNT(*) FROM blocklist WHERE release_guid = ?;

-- name: IsBlocklistedByTitle :one
SELECT COUNT(*) FROM blocklist WHERE release_title = ?;

-- name: ListBlocklist :many
SELECT b.*, s.title AS series_title
FROM blocklist b JOIN series s ON s.id = b.series_id
ORDER BY b.added_at DESC
LIMIT ? OFFSET ?;

-- name: CountBlocklist :one
SELECT COUNT(*) FROM blocklist;

-- name: DeleteBlocklistEntry :exec
DELETE FROM blocklist WHERE id = ?;

-- name: ClearBlocklist :exec
DELETE FROM blocklist;
