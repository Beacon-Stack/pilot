-- name: CreateLibrary :one
INSERT INTO libraries (
    id, name, root_path, default_quality_profile_id,
    naming_format, folder_format, min_free_space_gb, tags_json, created_at, updated_at
) VALUES (
    ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetLibrary :one
SELECT * FROM libraries WHERE id = ?;

-- name: ListLibraries :many
SELECT * FROM libraries ORDER BY name ASC;

-- name: UpdateLibrary :one
UPDATE libraries SET
    name                        = ?,
    root_path                   = ?,
    default_quality_profile_id  = ?,
    naming_format               = ?,
    folder_format               = ?,
    min_free_space_gb           = ?,
    tags_json                   = ?,
    updated_at                  = ?
WHERE id = ?
RETURNING *;

-- name: DeleteLibrary :exec
DELETE FROM libraries WHERE id = ?;

-- name: CountSeriesInLibrary :one
SELECT COUNT(*) FROM series WHERE library_id = ?;
