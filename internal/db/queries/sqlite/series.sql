-- name: CreateSeries :one
INSERT INTO series (
    id, tmdb_id, imdb_id, title, sort_title, year,
    overview, runtime_minutes, genres_json, poster_url, fanart_url,
    status, series_type, monitor_type, network, air_time, certification,
    monitored, library_id, quality_profile_id, path,
    added_at, updated_at, metadata_refreshed_at, alternate_titles,
    tvdb_id
) VALUES (
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?
)
RETURNING *;

-- name: GetSeries :one
SELECT * FROM series WHERE id = ?;

-- name: GetSeriesByTMDBID :one
SELECT * FROM series WHERE tmdb_id = ?;

-- name: ListSeries :many
SELECT * FROM series ORDER BY sort_title ASC LIMIT ? OFFSET ?;

-- name: ListSeriesByLibrary :many
SELECT * FROM series WHERE library_id = ? ORDER BY sort_title ASC LIMIT ? OFFSET ?;

-- name: CountSeries :one
SELECT COUNT(*) FROM series;

-- name: CountSeriesByLibrary :one
SELECT COUNT(*) FROM series WHERE library_id = ?;

-- name: ListMonitoredSeries :many
SELECT * FROM series WHERE monitored = TRUE ORDER BY sort_title ASC;

-- name: UpdateSeries :one
UPDATE series SET
    title              = ?,
    monitored          = ?,
    library_id         = ?,
    quality_profile_id = ?,
    series_type        = ?,
    path               = ?,
    updated_at         = ?
WHERE id = ?
RETURNING *;

-- name: UpdateSeriesMetadata :one
UPDATE series SET
    imdb_id               = ?,
    title                 = ?,
    sort_title            = ?,
    year                  = ?,
    overview              = ?,
    runtime_minutes       = ?,
    genres_json           = ?,
    poster_url            = ?,
    fanart_url            = ?,
    status                = ?,
    network               = ?,
    air_time              = ?,
    certification         = ?,
    metadata_refreshed_at = ?,
    updated_at            = ?,
    alternate_titles      = ?,
    tvdb_id               = ?
WHERE id = ?
RETURNING *;

-- name: DeleteSeries :exec
DELETE FROM series WHERE id = ?;

-- name: ListAllTMDBIDs :many
SELECT tmdb_id FROM series;

-- name: ListSeriesMissingExternalIDs :many
-- Series whose IMDb/TVDB ids haven't been looked up yet (NULL; a series
-- TMDB has none for is stored as '' / 0 and not asked about again).
SELECT id, tmdb_id FROM series WHERE tvdb_id IS NULL OR imdb_id IS NULL;

-- name: SetSeriesExternalIDs :exec
UPDATE series SET imdb_id = ?, tvdb_id = ? WHERE id = ?;
