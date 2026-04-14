-- name: GetMediaManagement :one
SELECT * FROM media_management WHERE id = 1;

-- name: UpdateMediaManagement :one
UPDATE media_management
SET rename_episodes            = $1,
    standard_episode_format    = $2,
    daily_episode_format       = $3,
    anime_episode_format       = $4,
    series_folder_format       = $5,
    season_folder_format       = $6,
    colon_replacement          = $7,
    import_extra_files         = $8,
    extra_file_extensions      = $9,
    unmonitor_deleted_episodes = $10
WHERE id = 1
RETURNING *;
