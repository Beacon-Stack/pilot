-- name: CreateImportListConfig :one
INSERT INTO import_list_configs (
    id, name, kind, enabled, settings, search_on_add, monitor,
    monitor_type, quality_profile_id, library_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetImportListConfig :one
SELECT * FROM import_list_configs WHERE id = ?;

-- name: ListImportListConfigs :many
SELECT * FROM import_list_configs ORDER BY name ASC;

-- name: ListEnabledImportLists :many
SELECT * FROM import_list_configs WHERE enabled = 1 ORDER BY name ASC;

-- name: UpdateImportListConfig :one
UPDATE import_list_configs SET
    name               = ?,
    kind               = ?,
    enabled            = ?,
    settings           = ?,
    search_on_add      = ?,
    monitor            = ?,
    monitor_type       = ?,
    quality_profile_id = ?,
    library_id         = ?,
    updated_at         = ?
WHERE id = ?
RETURNING *;

-- name: DeleteImportListConfig :exec
DELETE FROM import_list_configs WHERE id = ?;

-- name: CreateImportExclusion :one
INSERT INTO import_exclusions (id, tmdb_id, title, year, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetImportExclusionByTMDBID :one
SELECT * FROM import_exclusions WHERE tmdb_id = ?;

-- name: ListImportExclusions :many
SELECT * FROM import_exclusions ORDER BY title ASC;

-- name: DeleteImportExclusion :exec
DELETE FROM import_exclusions WHERE id = ?;

-- name: ListExcludedTMDBIDs :many
SELECT tmdb_id FROM import_exclusions;
