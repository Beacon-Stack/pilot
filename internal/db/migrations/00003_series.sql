-- +goose Up

CREATE TABLE series (
    id                    TEXT NOT NULL PRIMARY KEY,
    tmdb_id               INTEGER NOT NULL UNIQUE,
    imdb_id               TEXT,
    title                 TEXT NOT NULL,
    sort_title            TEXT NOT NULL,
    year                  INTEGER NOT NULL,
    overview              TEXT NOT NULL DEFAULT '',
    runtime_minutes       INTEGER,
    genres_json           TEXT NOT NULL DEFAULT '[]',
    poster_url            TEXT,
    fanart_url            TEXT,
    -- "continuing", "ended", "upcoming"
    status                TEXT NOT NULL DEFAULT 'continuing',
    -- "standard", "daily", "anime"
    series_type           TEXT NOT NULL DEFAULT 'standard',
    -- "all", "future", "missing", "existing", "pilot", "first_season", "last_season", "none"
    monitor_type          TEXT NOT NULL DEFAULT 'all',
    network               TEXT,
    air_time              TEXT,
    certification         TEXT,
    monitored             INTEGER NOT NULL DEFAULT 1,
    library_id            TEXT NOT NULL REFERENCES libraries(id),
    quality_profile_id    TEXT NOT NULL REFERENCES quality_profiles(id),
    path                  TEXT,
    added_at              TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    metadata_refreshed_at TEXT
);

CREATE INDEX idx_series_library_id ON series(library_id);
CREATE INDEX idx_series_status ON series(status);
CREATE INDEX idx_series_monitored ON series(monitored);

CREATE TABLE seasons (
    id            TEXT NOT NULL PRIMARY KEY,
    series_id     TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    season_number INTEGER NOT NULL,
    monitored     INTEGER NOT NULL DEFAULT 1,
    UNIQUE(series_id, season_number)
);

CREATE INDEX idx_seasons_series_id ON seasons(series_id);

CREATE TABLE episodes (
    id              TEXT NOT NULL PRIMARY KEY,
    series_id       TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    season_id       TEXT NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    season_number   INTEGER NOT NULL,
    episode_number  INTEGER NOT NULL,
    absolute_number INTEGER,
    air_date        TEXT,
    title           TEXT NOT NULL DEFAULT '',
    overview        TEXT NOT NULL DEFAULT '',
    monitored       INTEGER NOT NULL DEFAULT 1,
    has_file        INTEGER NOT NULL DEFAULT 0,
    UNIQUE(series_id, season_number, episode_number)
);

CREATE INDEX idx_episodes_series_id ON episodes(series_id);
CREATE INDEX idx_episodes_season_id ON episodes(season_id);
CREATE INDEX idx_episodes_air_date ON episodes(air_date);
CREATE INDEX idx_episodes_monitored_has_file ON episodes(monitored, has_file);

-- +goose Down

DROP TABLE IF EXISTS episodes;
DROP TABLE IF EXISTS seasons;
DROP TABLE IF EXISTS series;
