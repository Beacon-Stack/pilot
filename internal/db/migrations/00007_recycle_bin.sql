-- +goose Up

-- Recycle bin (as in Sonarr's Media Management > File Management).
--
--   recycle_bin_path          folder that replaced and deleted episode files
--                             are moved to; '' = delete them permanently
--   recycle_bin_cleanup_days  files older than this in the recycle bin are
--                             deleted; 0 = never
ALTER TABLE media_management ADD COLUMN recycle_bin_path TEXT NOT NULL DEFAULT '';
ALTER TABLE media_management ADD COLUMN recycle_bin_cleanup_days INTEGER NOT NULL DEFAULT 7;

-- +goose Down
ALTER TABLE media_management DROP COLUMN recycle_bin_cleanup_days;
ALTER TABLE media_management DROP COLUMN recycle_bin_path;
