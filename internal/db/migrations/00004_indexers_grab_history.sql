-- +goose Up

CREATE TABLE indexer_configs (
    id         TEXT NOT NULL PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    priority   INTEGER NOT NULL DEFAULT 25,
    settings   TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE grab_history (
    id                  TEXT NOT NULL PRIMARY KEY,
    series_id           TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    episode_id          TEXT REFERENCES episodes(id) ON DELETE SET NULL,
    season_number       INTEGER,
    indexer_id          TEXT,
    release_guid        TEXT NOT NULL,
    release_title       TEXT NOT NULL,
    release_source      TEXT NOT NULL DEFAULT 'unknown',
    release_resolution  TEXT NOT NULL DEFAULT 'unknown',
    release_codec       TEXT NOT NULL DEFAULT 'unknown',
    release_hdr         TEXT NOT NULL DEFAULT 'none',
    protocol            TEXT NOT NULL DEFAULT 'unknown',
    size                INTEGER NOT NULL DEFAULT 0,
    download_client_id  TEXT,
    client_item_id      TEXT,
    download_status     TEXT NOT NULL DEFAULT 'pending',
    downloaded_bytes    INTEGER NOT NULL DEFAULT 0,
    score_breakdown     TEXT,
    grabbed_at          TEXT NOT NULL
);

CREATE INDEX idx_grab_history_series_id ON grab_history(series_id);
CREATE INDEX idx_grab_history_episode_id ON grab_history(episode_id);
CREATE INDEX idx_grab_history_grabbed_at ON grab_history(grabbed_at DESC);
CREATE INDEX idx_grab_history_download_status ON grab_history(download_status);

-- +goose Down

DROP TABLE IF EXISTS grab_history;
DROP TABLE IF EXISTS indexer_configs;
