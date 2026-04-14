-- name: CreateLibrary :one
INSERT INTO libraries (
    id, name, root_path, default_quality_profile_id,
    naming_format, folder_format, min_free_space_gb, tags_json, created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: GetLibrary :one
SELECT * FROM libraries WHERE id = $1;

-- name: ListLibraries :many
SELECT * FROM libraries ORDER BY name ASC;

-- name: UpdateLibrary :one
UPDATE libraries SET
    name                        = $1,
    root_path                   = $2,
    default_quality_profile_id  = $3,
    naming_format               = $4,
    folder_format               = $5,
    min_free_space_gb           = $6,
    tags_json                   = $7,
    updated_at                  = $8
WHERE id = $9
RETURNING *;

-- name: DeleteLibrary :exec
DELETE FROM libraries WHERE id = $1;

-- name: CountSeriesInLibrary :one
SELECT COUNT(*) FROM series WHERE library_id = $1;
