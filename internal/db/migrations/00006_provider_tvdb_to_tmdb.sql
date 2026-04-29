-- +goose Up

-- Rename any pre-existing TVDB provider override to TMDB. Pilot was
-- always calling TMDB internally; the "tvdb" naming was a copy-paste
-- holdover from Prism. The settings row is the only stored artifact
-- that carries the old name. ON CONFLICT lets us safely re-run if
-- someone happened to set both (very unlikely in practice).
INSERT INTO settings (key, value, updated_at)
SELECT 'provider.tmdb.api_key', value, updated_at
FROM settings
WHERE key = 'provider.tvdb.api_key'
ON CONFLICT (key) DO UPDATE SET
    value      = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at;

DELETE FROM settings WHERE key = 'provider.tvdb.api_key';

-- +goose Down

-- Reverse the rename. Same idempotent insert+delete pattern.
INSERT INTO settings (key, value, updated_at)
SELECT 'provider.tvdb.api_key', value, updated_at
FROM settings
WHERE key = 'provider.tmdb.api_key'
ON CONFLICT (key) DO UPDATE SET
    value      = EXCLUDED.value,
    updated_at = EXCLUDED.updated_at;

DELETE FROM settings WHERE key = 'provider.tmdb.api_key';
