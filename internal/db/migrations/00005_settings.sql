-- +goose Up

-- Shared key-value store for runtime settings not owned by another table.
-- Initial users: provider API key overrides (provider.tvdb.api_key,
-- provider.trakt.client_id) that let operators rotate the baked-in
-- defaults via the Settings UI.
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE settings;
