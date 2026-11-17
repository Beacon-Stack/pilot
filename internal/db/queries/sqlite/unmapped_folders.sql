-- name: InsertUnmappedFolder :exec
INSERT INTO unmapped_folders (id, library_id, path, name, guess_tmdb_id, guess_title, guess_year, guess_poster_path, ignored, first_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, FALSE, ?)
ON CONFLICT(path) DO NOTHING;

-- name: GetUnmappedFolder :one
SELECT * FROM unmapped_folders WHERE id = ?;

-- name: ListUnmappedFoldersByLibrary :many
SELECT * FROM unmapped_folders WHERE library_id = ? ORDER BY name ASC;

-- name: ListUnmappedFolders :many
SELECT * FROM unmapped_folders ORDER BY name ASC;

-- name: SetUnmappedFolderIgnored :exec
UPDATE unmapped_folders SET ignored = ? WHERE id = ?;

-- name: DeleteUnmappedFolder :exec
DELETE FROM unmapped_folders WHERE id = ?;
