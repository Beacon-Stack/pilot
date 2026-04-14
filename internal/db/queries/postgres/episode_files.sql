-- name: CreateEpisodeFile :one
INSERT INTO episode_files (id, episode_id, series_id, path, size_bytes, quality_json, imported_at, indexed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetEpisodeFile :one
SELECT * FROM episode_files WHERE id = $1;

-- name: GetEpisodeFileByPath :one
SELECT * FROM episode_files WHERE path = $1;

-- name: ListEpisodeFilesBySeriesID :many
SELECT * FROM episode_files WHERE series_id = $1 ORDER BY path ASC;

-- name: ListEpisodeFilesByEpisodeID :many
SELECT * FROM episode_files WHERE episode_id = $1 ORDER BY path ASC;

-- name: DeleteEpisodeFile :exec
DELETE FROM episode_files WHERE id = $1;

-- name: DeleteEpisodeFilesBySeriesID :exec
DELETE FROM episode_files WHERE series_id = $1;

-- name: CountEpisodeFilesBySeriesID :one
SELECT COUNT(*) FROM episode_files WHERE series_id = $1;

-- name: ListAllEpisodeFilePaths :many
SELECT path FROM episode_files;

-- name: UpdateEpisodeFilePath :exec
UPDATE episode_files SET path = $1 WHERE id = $2;
