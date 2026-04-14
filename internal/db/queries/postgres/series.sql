-- name: CreateSeries :one
INSERT INTO series (
    id, tmdb_id, imdb_id, title, sort_title, year,
    overview, runtime_minutes, genres_json, poster_url, fanart_url,
    status, series_type, monitor_type, network, air_time, certification,
    monitored, library_id, quality_profile_id, path,
    added_at, updated_at, metadata_refreshed_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15, $16, $17,
    $18, $19, $20, $21,
    $22, $23, $24
)
RETURNING *;

-- name: GetSeries :one
SELECT * FROM series WHERE id = $1;

-- name: GetSeriesByTMDBID :one
SELECT * FROM series WHERE tmdb_id = $1;

-- name: ListSeries :many
SELECT * FROM series ORDER BY sort_title ASC LIMIT $1 OFFSET $2;

-- name: ListSeriesByLibrary :many
SELECT * FROM series WHERE library_id = $1 ORDER BY sort_title ASC LIMIT $2 OFFSET $3;

-- name: CountSeries :one
SELECT COUNT(*) FROM series;

-- name: CountSeriesByLibrary :one
SELECT COUNT(*) FROM series WHERE library_id = $1;

-- name: ListMonitoredSeries :many
SELECT * FROM series WHERE monitored = TRUE ORDER BY sort_title ASC;

-- name: UpdateSeries :one
UPDATE series SET
    title              = $1,
    monitored          = $2,
    library_id         = $3,
    quality_profile_id = $4,
    series_type        = $5,
    path               = $6,
    updated_at         = $7
WHERE id = $8
RETURNING *;

-- name: UpdateSeriesMetadata :one
UPDATE series SET
    imdb_id               = $1,
    title                 = $2,
    sort_title            = $3,
    year                  = $4,
    overview              = $5,
    runtime_minutes       = $6,
    genres_json           = $7,
    poster_url            = $8,
    fanart_url            = $9,
    status                = $10,
    network               = $11,
    air_time              = $12,
    certification         = $13,
    metadata_refreshed_at = $14,
    updated_at            = $15
WHERE id = $16
RETURNING *;

-- name: DeleteSeries :exec
DELETE FROM series WHERE id = $1;

-- name: ListAllTMDBIDs :many
SELECT tmdb_id FROM series;
