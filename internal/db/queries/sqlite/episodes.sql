-- name: CreateEpisode :one
INSERT INTO episodes (
    id, series_id, season_id, season_number, episode_number,
    absolute_number, air_date, title, overview, monitored, has_file,
    still_path, runtime_minutes
) VALUES (
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?
)
RETURNING *;

-- name: GetEpisode :one
SELECT * FROM episodes WHERE id = ?;

-- name: ListEpisodesBySeasonID :many
SELECT * FROM episodes WHERE season_id = ? ORDER BY episode_number ASC;

-- name: ListEpisodesBySeriesID :many
SELECT * FROM episodes WHERE series_id = ? ORDER BY season_number ASC, episode_number ASC;

-- name: UpdateEpisode :one
UPDATE episodes SET
    title    = ?,
    overview = ?,
    air_date = ?,
    has_file = ?,
    still_path = ?,
    runtime_minutes = ?
WHERE id = ?
RETURNING *;

-- name: UpdateEpisodeMonitored :exec
UPDATE episodes SET monitored = ? WHERE id = ?;

-- name: UpdateEpisodesMonitoredBySeason :exec
UPDATE episodes SET monitored = ? WHERE season_id = ?;

-- name: CountEpisodesBySeriesID :one
SELECT COUNT(*) FROM episodes WHERE series_id = ?;

-- name: CountEpisodesWithFileBySeriesID :one
SELECT COUNT(*) FROM episodes WHERE series_id = ? AND has_file = 1;

-- name: CountMissingEpisodes :one
SELECT COUNT(*) FROM episodes WHERE monitored = 1 AND has_file = 0 AND air_date IS NOT NULL AND air_date <= date('now');

-- name: ListMissingEpisodes :many
SELECT * FROM episodes WHERE monitored = 1 AND has_file = 0 AND air_date IS NOT NULL AND air_date <= date('now')
ORDER BY air_date DESC LIMIT ? OFFSET ?;

-- name: DeleteEpisodesBySeriesID :exec
DELETE FROM episodes WHERE series_id = ?;

-- name: ListEpisodesByAirDateRange :many
SELECT e.id, e.series_id, e.season_id, e.season_number, e.episode_number, e.absolute_number, e.air_date, e.title, e.overview, e.monitored, e.has_file, e.still_path, e.runtime_minutes, s.title as series_title
FROM episodes e
JOIN series s ON s.id = e.series_id
WHERE e.air_date >= ? AND e.air_date <= ?
ORDER BY e.air_date ASC, s.title ASC, e.episode_number ASC;

-- name: ListMissingEpisodesWithSeries :many
SELECT e.id, e.series_id, e.season_id, e.season_number, e.episode_number, e.absolute_number, e.air_date, e.title, e.overview, e.monitored, e.has_file, e.still_path, e.runtime_minutes, s.title as series_title
FROM episodes e
JOIN series s ON s.id = e.series_id
WHERE e.monitored = 1 AND e.has_file = 0 AND e.air_date IS NOT NULL AND e.air_date <= date('now')
ORDER BY e.air_date DESC
LIMIT ? OFFSET ?;

