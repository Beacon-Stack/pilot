-- name: InsertStatsSnapshot :exec
INSERT INTO stats_snapshots (
    id, total_series, total_episodes, monitored_episodes,
    with_file, missing, total_size_bytes, snapshot_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListStatsSnapshots :many
SELECT * FROM stats_snapshots
ORDER BY snapshot_at DESC
LIMIT ?;

-- name: LatestStatsSnapshot :one
SELECT * FROM stats_snapshots
ORDER BY snapshot_at DESC
LIMIT 1;

-- name: CountAllEpisodes :one
SELECT COUNT(*) FROM episodes;

-- name: CountMonitoredEpisodes :one
SELECT COUNT(*) FROM episodes WHERE monitored = TRUE;

-- name: CountEpisodesWithFile :one
SELECT COUNT(*) FROM episodes WHERE has_file = TRUE;

-- name: SumEpisodeFileSize :one
SELECT COALESCE(SUM(size_bytes), 0) FROM episode_files;

-- name: CountEpisodeFiles :one
SELECT COUNT(*) FROM episode_files;

-- name: ListEpisodeFileQualities :many
SELECT quality_json FROM episode_files;

-- name: ListEpisodeFileQualitiesWithSeriesIDs :many
SELECT series_id, quality_json FROM episode_files;

-- name: GetGrabStats :one
SELECT
    COUNT(*)                                                                       AS total_grabs,
    COALESCE(SUM(CASE WHEN download_status = 'completed' THEN 1 ELSE 0 END), 0)  AS successful,
    COALESCE(SUM(CASE WHEN download_status = 'failed' THEN 1 ELSE 0 END), 0)     AS failed
FROM grab_history;

-- name: GetTopIndexers :many
SELECT
    gh.indexer_id,
    COALESCE(ic.name, gh.indexer_id)                                                  AS indexer_name,
    COUNT(*)                                                                          AS grab_count,
    COALESCE(SUM(CASE WHEN gh.download_status = 'completed' THEN 1 ELSE 0 END), 0)  AS success_count
FROM grab_history gh
LEFT JOIN indexer_configs ic ON ic.id = gh.indexer_id
WHERE gh.indexer_id IS NOT NULL AND gh.indexer_id != ''
GROUP BY gh.indexer_id, ic.name
ORDER BY grab_count DESC
LIMIT 10;

-- name: GetSeriesYearDistribution :many
SELECT year, COUNT(*) AS count
FROM series
WHERE year > 0
GROUP BY year
ORDER BY year ASC;

-- name: ListSeriesGenresJSON :many
SELECT genres_json FROM series WHERE genres_json IS NOT NULL AND genres_json != '[]';
