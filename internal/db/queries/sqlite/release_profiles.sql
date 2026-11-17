-- name: CreateReleaseProfile :one
INSERT INTO release_profiles (id, name, enabled, required_json, ignored_json, indexer_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetReleaseProfile :one
SELECT * FROM release_profiles WHERE id = ?;

-- name: ListReleaseProfiles :many
SELECT * FROM release_profiles ORDER BY name ASC;

-- name: ListEnabledReleaseProfiles :many
SELECT * FROM release_profiles WHERE enabled = TRUE ORDER BY name ASC;

-- name: UpdateReleaseProfile :one
UPDATE release_profiles SET
    name          = ?,
    enabled       = ?,
    required_json = ?,
    ignored_json  = ?,
    indexer_id    = ?,
    updated_at    = ?
WHERE id = ?
RETURNING *;

-- name: DeleteReleaseProfile :exec
DELETE FROM release_profiles WHERE id = ?;
