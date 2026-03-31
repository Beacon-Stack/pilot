-- name: CreateEpisodeFile :one
INSERT INTO episode_files (id, episode_id, series_id, path, size_bytes, quality_json, imported_at, indexed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetEpisodeFile :one
SELECT * FROM episode_files WHERE id = ?;

-- name: GetEpisodeFileByPath :one
SELECT * FROM episode_files WHERE path = ?;

-- name: ListEpisodeFilesBySeriesID :many
SELECT * FROM episode_files WHERE series_id = ? ORDER BY path ASC;

-- name: ListEpisodeFilesByEpisodeID :many
SELECT * FROM episode_files WHERE episode_id = ? ORDER BY path ASC;

-- name: DeleteEpisodeFile :exec
DELETE FROM episode_files WHERE id = ?;

-- name: DeleteEpisodeFilesBySeriesID :exec
DELETE FROM episode_files WHERE series_id = ?;

-- name: CountEpisodeFilesBySeriesID :one
SELECT COUNT(*) FROM episode_files WHERE series_id = ?;

-- name: ListAllEpisodeFilePaths :many
SELECT path FROM episode_files;

-- name: UpdateEpisodeFilePath :exec
UPDATE episode_files SET path = ? WHERE id = ?;
