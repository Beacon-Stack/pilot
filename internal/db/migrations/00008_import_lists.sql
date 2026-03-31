-- +goose Up

CREATE TABLE import_list_configs (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    kind                TEXT NOT NULL,
    enabled             INTEGER NOT NULL DEFAULT 1,
    settings            TEXT NOT NULL DEFAULT '{}',
    search_on_add       INTEGER NOT NULL DEFAULT 0,
    monitor             INTEGER NOT NULL DEFAULT 1,
    monitor_type        TEXT NOT NULL DEFAULT 'all',
    quality_profile_id  TEXT NOT NULL DEFAULT '',
    library_id          TEXT NOT NULL DEFAULT '',
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE TABLE import_exclusions (
    id         TEXT PRIMARY KEY,
    tmdb_id    INTEGER NOT NULL UNIQUE,
    title      TEXT NOT NULL DEFAULT '',
    year       INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

-- +goose Down

DROP TABLE IF EXISTS import_exclusions;
DROP TABLE IF EXISTS import_list_configs;
