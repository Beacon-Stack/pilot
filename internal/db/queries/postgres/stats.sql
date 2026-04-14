-- name: InsertStatsSnapshot :exec
INSERT INTO stats_snapshots (
    id, total_series, total_episodes, monitored_episodes,
    with_file, missing, total_size_bytes, snapshot_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListStatsSnapshots :many
SELECT * FROM stats_snapshots
ORDER BY snapshot_at DESC
LIMIT $1;

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
