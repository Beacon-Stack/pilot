-- +goose Up

CREATE TABLE activity_log (
    id         TEXT PRIMARY KEY,
    type       TEXT NOT NULL,
    category   TEXT NOT NULL,
    series_id  TEXT,
    title      TEXT NOT NULL,
    detail     TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_activity_log_created   ON activity_log(created_at DESC);
CREATE INDEX idx_activity_log_category  ON activity_log(category);
CREATE INDEX idx_activity_log_series    ON activity_log(series_id);

CREATE TABLE stats_snapshots (
    id                  TEXT PRIMARY KEY,
    total_series        INTEGER NOT NULL DEFAULT 0,
    total_episodes      INTEGER NOT NULL DEFAULT 0,
    monitored_episodes  INTEGER NOT NULL DEFAULT 0,
    with_file           INTEGER NOT NULL DEFAULT 0,
    missing             INTEGER NOT NULL DEFAULT 0,
    total_size_bytes    INTEGER NOT NULL DEFAULT 0,
    snapshot_at         TEXT NOT NULL
);
CREATE INDEX idx_stats_snapshots_at ON stats_snapshots(snapshot_at DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_stats_snapshots_at;
DROP TABLE IF EXISTS stats_snapshots;

DROP INDEX IF EXISTS idx_activity_log_series;
DROP INDEX IF EXISTS idx_activity_log_category;
DROP INDEX IF EXISTS idx_activity_log_created;
DROP TABLE IF EXISTS activity_log;
