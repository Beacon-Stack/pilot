-- name: CreateEpisode :one
INSERT INTO episodes (
    id, series_id, season_id, season_number, episode_number,
    absolute_number, air_date, title, overview, monitored, has_file,
    still_path, runtime_minutes
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10, $11,
    $12, $13
)
RETURNING *;

-- name: GetEpisode :one
SELECT * FROM episodes WHERE id = $1;

-- name: ListEpisodesBySeasonID :many
SELECT * FROM episodes WHERE season_id = $1 ORDER BY episode_number ASC;

-- name: ListEpisodesBySeriesID :many
SELECT * FROM episodes WHERE series_id = $1 ORDER BY season_number ASC, episode_number ASC;

-- name: UpdateEpisode :one
UPDATE episodes SET
    title    = $1,
    overview = $2,
    air_date = $3,
    has_file = $4,
    still_path = $5,
    runtime_minutes = $6
WHERE id = $7
RETURNING *;

-- name: UpdateEpisodeMonitored :exec
UPDATE episodes SET monitored = $1 WHERE id = $2;

-- name: UpdateEpisodeAbsoluteNumber :exec
-- Backfill or correct the absolute episode number. Used by the refresh
-- path when a series is newly flagged as anime — its existing rows have
-- absolute_number = NULL and need to be filled in retroactively.
UPDATE episodes SET absolute_number = $1 WHERE id = $2;

-- name: UpdateEpisodesMonitoredBySeason :exec
UPDATE episodes SET monitored = $1 WHERE season_id = $2;

-- name: CountEpisodesBySeriesID :one
SELECT COUNT(*) FROM episodes WHERE series_id = $1;

-- name: CountEpisodesWithFileBySeriesID :one
SELECT COUNT(*) FROM episodes WHERE series_id = $1 AND has_file = TRUE;

-- name: CountMissingEpisodes :one
SELECT COUNT(*) FROM episodes WHERE monitored = TRUE AND has_file = FALSE AND air_date IS NOT NULL AND air_date <= to_char(CURRENT_DATE, 'YYYY-MM-DD');

-- name: ListMissingEpisodes :many
SELECT * FROM episodes WHERE monitored = TRUE AND has_file = FALSE AND air_date IS NOT NULL AND air_date <= to_char(CURRENT_DATE, 'YYYY-MM-DD')
ORDER BY air_date DESC LIMIT $1 OFFSET $2;

-- name: DeleteEpisodesBySeriesID :exec
DELETE FROM episodes WHERE series_id = $1;

-- name: ListEpisodesByAirDateRange :many
SELECT e.id, e.series_id, e.season_id, e.season_number, e.episode_number, e.absolute_number, e.air_date, e.title, e.overview, e.monitored, e.has_file, e.still_path, e.runtime_minutes, s.title as series_title
FROM episodes e
JOIN series s ON s.id = e.series_id
WHERE e.air_date >= $1 AND e.air_date <= $2
ORDER BY e.air_date ASC, s.title ASC, e.episode_number ASC;

-- name: ListMissingEpisodesWithSeries :many
SELECT e.id, e.series_id, e.season_id, e.season_number, e.episode_number, e.absolute_number, e.air_date, e.title, e.overview, e.monitored, e.has_file, e.still_path, e.runtime_minutes, s.title as series_title
FROM episodes e
JOIN series s ON s.id = e.series_id
WHERE e.monitored = TRUE AND e.has_file = FALSE AND e.air_date IS NOT NULL AND e.air_date <= to_char(CURRENT_DATE, 'YYYY-MM-DD')
ORDER BY e.air_date DESC
LIMIT $1 OFFSET $2;
