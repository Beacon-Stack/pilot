-- name: CreateRemotePathMapping :one
INSERT INTO remote_path_mappings (id, download_client_id, remote_path, local_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRemotePathMapping :one
SELECT * FROM remote_path_mappings WHERE id = ?;

-- name: ListRemotePathMappings :many
SELECT * FROM remote_path_mappings ORDER BY remote_path ASC, local_path ASC;

-- name: UpdateRemotePathMapping :one
UPDATE remote_path_mappings SET
    download_client_id = ?,
    remote_path        = ?,
    local_path         = ?,
    updated_at         = ?
WHERE id = ?
RETURNING *;

-- name: DeleteRemotePathMapping :exec
DELETE FROM remote_path_mappings WHERE id = ?;
