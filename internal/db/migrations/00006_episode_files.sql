-- +goose Up

CREATE TABLE episode_files (
    id          TEXT NOT NULL PRIMARY KEY,
    episode_id  TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    series_id   TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    path        TEXT NOT NULL UNIQUE,
    size_bytes  INTEGER NOT NULL,
    quality_json TEXT NOT NULL DEFAULT '{}',
    imported_at TEXT NOT NULL,
    indexed_at  TEXT NOT NULL
);

CREATE INDEX idx_episode_files_episode_id ON episode_files(episode_id);
CREATE INDEX idx_episode_files_series_id ON episode_files(series_id);

-- +goose Down

DROP TABLE IF EXISTS episode_files;
