-- +goose Up

-- ── Quality profiles ─────────────────────────────────────────────────────────

CREATE TABLE quality_profiles (
    id                      TEXT NOT NULL PRIMARY KEY,
    name                    TEXT NOT NULL,
    cutoff_json             TEXT NOT NULL DEFAULT '{}',
    qualities_json          TEXT NOT NULL DEFAULT '[]',
    upgrade_allowed         BOOLEAN NOT NULL DEFAULT TRUE,
    upgrade_until_json      TEXT,
    min_custom_format_score INTEGER NOT NULL DEFAULT 0,
    upgrade_until_cf_score  INTEGER NOT NULL DEFAULT 0,
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);

-- ── Libraries ────────────────────────────────────────────────────────────────

CREATE TABLE libraries (
    id                          TEXT NOT NULL PRIMARY KEY,
    name                        TEXT NOT NULL,
    root_path                   TEXT NOT NULL,
    default_quality_profile_id  TEXT NOT NULL REFERENCES quality_profiles(id),
    naming_format               TEXT,
    folder_format               TEXT,
    min_free_space_gb           INTEGER NOT NULL DEFAULT 5,
    tags_json                   TEXT NOT NULL DEFAULT '[]',
    created_at                  TEXT NOT NULL,
    updated_at                  TEXT NOT NULL
);

-- ── Series ───────────────────────────────────────────────────────────────────

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
    status                TEXT NOT NULL DEFAULT 'continuing',
    series_type           TEXT NOT NULL DEFAULT 'standard',
    monitor_type          TEXT NOT NULL DEFAULT 'all',
    network               TEXT,
    air_time              TEXT,
    certification         TEXT,
    monitored             BOOLEAN NOT NULL DEFAULT TRUE,
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

-- ── Seasons ──────────────────────────────────────────────────────────────────

CREATE TABLE seasons (
    id            TEXT NOT NULL PRIMARY KEY,
    series_id     TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    season_number INTEGER NOT NULL,
    monitored     BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE(series_id, season_number)
);

CREATE INDEX idx_seasons_series_id ON seasons(series_id);

-- ── Episodes ─────────────────────────────────────────────────────────────────

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
    monitored       BOOLEAN NOT NULL DEFAULT TRUE,
    has_file        BOOLEAN NOT NULL DEFAULT FALSE,
    still_path      TEXT NOT NULL DEFAULT '',
    runtime_minutes INTEGER NOT NULL DEFAULT 0,
    UNIQUE(series_id, season_number, episode_number)
);

CREATE INDEX idx_episodes_series_id ON episodes(series_id);
CREATE INDEX idx_episodes_season_id ON episodes(season_id);
CREATE INDEX idx_episodes_air_date ON episodes(air_date);
CREATE INDEX idx_episodes_monitored_has_file ON episodes(monitored, has_file);

-- ── Indexer configs ──────────────────────────────────────────────────────────

CREATE TABLE indexer_configs (
    id         TEXT NOT NULL PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    priority   INTEGER NOT NULL DEFAULT 25,
    settings   TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- ── Grab history ─────────────────────────────────────────────────────────────

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

-- ── Download clients ─────────────────────────────────────────────────────────

CREATE TABLE download_client_configs (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    priority   INTEGER NOT NULL DEFAULT 25,
    settings   TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- ── Notifications ────────────────────────────────────────────────────────────

CREATE TABLE notification_configs (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    settings   TEXT NOT NULL DEFAULT '{}',
    on_events  TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- ── Media servers ────────────────────────────────────────────────────────────

CREATE TABLE media_server_configs (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    settings   TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- ── Blocklist ────────────────────────────────────────────────────────────────

CREATE TABLE blocklist (
    id            TEXT PRIMARY KEY,
    series_id     TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    episode_id    TEXT REFERENCES episodes(id) ON DELETE SET NULL,
    release_guid  TEXT NOT NULL,
    release_title TEXT NOT NULL,
    indexer_id    TEXT,
    protocol      TEXT NOT NULL DEFAULT '',
    size          INTEGER NOT NULL DEFAULT 0,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_blocklist_series_id ON blocklist(series_id);
CREATE INDEX idx_blocklist_episode_id ON blocklist(episode_id);
CREATE UNIQUE INDEX idx_blocklist_guid ON blocklist(release_guid);

-- ── Quality definitions ──────────────────────────────────────────────────────

CREATE TABLE quality_definitions (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    resolution     TEXT NOT NULL,
    source         TEXT NOT NULL,
    codec          TEXT NOT NULL,
    hdr            TEXT NOT NULL,
    min_size       REAL NOT NULL DEFAULT 0,
    max_size       REAL NOT NULL DEFAULT 0,
    preferred_size REAL NOT NULL DEFAULT 0,
    sort_order     INTEGER NOT NULL DEFAULT 0
);

INSERT INTO quality_definitions (id, name, resolution, source, codec, hdr, min_size, max_size, preferred_size, sort_order) VALUES
  ('sd-dvd-xvid-none',        'SD DVD',          'sd',    'dvd',    'xvid', 'none',  0,    3,   0,  10),
  ('sd-hdtv-x264-none',       'SD HDTV',         'sd',    'hdtv',   'x264', 'none',  0,    3,   0,  20),
  ('720p-hdtv-x264-none',     '720p HDTV',       '720p',  'hdtv',   'x264', 'none',  2,   20,   0,  30),
  ('720p-webdl-x264-none',    '720p WEBDL',      '720p',  'webdl',  'x264', 'none',  2,   20,   0,  40),
  ('720p-webrip-x264-none',   '720p WEBRip',     '720p',  'webrip', 'x264', 'none',  2,   20,   0,  50),
  ('720p-bluray-x264-none',   '720p Bluray',     '720p',  'bluray', 'x264', 'none',  2,   30,   0,  60),
  ('1080p-hdtv-x264-none',    '1080p HDTV',      '1080p', 'hdtv',   'x264', 'none',  4,   40,   0,  70),
  ('1080p-webdl-x264-none',   '1080p WEBDL',     '1080p', 'webdl',  'x264', 'none',  4,   40,   0,  80),
  ('1080p-webrip-x265-none',  '1080p WEBRip',    '1080p', 'webrip', 'x265', 'none',  4,   40,   0,  90),
  ('1080p-bluray-x265-none',  '1080p Bluray',    '1080p', 'bluray', 'x265', 'none',  4,   95,   0, 100),
  ('1080p-remux-x265-none',   '1080p Remux',     '1080p', 'remux',  'x265', 'none', 17,  400,   0, 110),
  ('2160p-webdl-x265-hdr10',  '2160p WEBDL HDR', '2160p', 'webdl',  'x265', 'hdr10', 15, 250,   0, 120),
  ('2160p-bluray-x265-hdr10', '2160p Bluray HDR','2160p', 'bluray', 'x265', 'hdr10', 15, 250,   0, 130),
  ('2160p-remux-x265-hdr10',  '2160p Remux HDR', '2160p', 'remux',  'x265', 'hdr10', 35, 800,   0, 140);

-- ── Media management ─────────────────────────────────────────────────────────

CREATE TABLE media_management (
    id                        INTEGER PRIMARY KEY CHECK (id = 1),
    rename_episodes           BOOLEAN NOT NULL DEFAULT TRUE,
    standard_episode_format   TEXT    NOT NULL DEFAULT '{Series Title} - S{season:00}E{episode:00} - {Episode Title} {Quality Full}',
    daily_episode_format      TEXT    NOT NULL DEFAULT '{Series Title} - {Air-Date} - {Episode Title} {Quality Full}',
    anime_episode_format      TEXT    NOT NULL DEFAULT '{Series Title} - S{season:00}E{episode:00} - {Episode Title} {Quality Full}',
    series_folder_format      TEXT    NOT NULL DEFAULT '{Series Title} ({Year})',
    season_folder_format      TEXT    NOT NULL DEFAULT 'Season {season:00}',
    colon_replacement         TEXT    NOT NULL DEFAULT 'space-dash',
    import_extra_files        BOOLEAN NOT NULL DEFAULT FALSE,
    extra_file_extensions     TEXT    NOT NULL DEFAULT 'srt,nfo',
    unmonitor_deleted_episodes BOOLEAN NOT NULL DEFAULT FALSE
);

INSERT INTO media_management (id) VALUES (1);

-- ── Episode files ────────────────────────────────────────────────────────────

CREATE TABLE episode_files (
    id          TEXT NOT NULL PRIMARY KEY,
    episode_id  TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    series_id   TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    path        TEXT NOT NULL UNIQUE,
    size_bytes  BIGINT NOT NULL,
    quality_json TEXT NOT NULL DEFAULT '{}',
    imported_at TEXT NOT NULL,
    indexed_at  TEXT NOT NULL
);

CREATE INDEX idx_episode_files_episode_id ON episode_files(episode_id);
CREATE INDEX idx_episode_files_series_id ON episode_files(series_id);

-- ── Activity log ─────────────────────────────────────────────────────────────

CREATE TABLE activity_log (
    id         TEXT PRIMARY KEY,
    type       TEXT NOT NULL,
    category   TEXT NOT NULL,
    series_id  TEXT,
    title      TEXT NOT NULL,
    detail     TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_activity_log_created ON activity_log(created_at DESC);
CREATE INDEX idx_activity_log_category ON activity_log(category);
CREATE INDEX idx_activity_log_series ON activity_log(series_id);

-- ── Stats snapshots ──────────────────────────────────────────────────────────

CREATE TABLE stats_snapshots (
    id                  TEXT PRIMARY KEY,
    total_series        INTEGER NOT NULL DEFAULT 0,
    total_episodes      INTEGER NOT NULL DEFAULT 0,
    monitored_episodes  INTEGER NOT NULL DEFAULT 0,
    with_file           INTEGER NOT NULL DEFAULT 0,
    missing             INTEGER NOT NULL DEFAULT 0,
    total_size_bytes    BIGINT NOT NULL DEFAULT 0,
    snapshot_at         TEXT NOT NULL
);

CREATE INDEX idx_stats_snapshots_at ON stats_snapshots(snapshot_at DESC);

-- ── Import list configs ──────────────────────────────────────────────────────

CREATE TABLE import_list_configs (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    kind                TEXT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    settings            TEXT NOT NULL DEFAULT '{}',
    search_on_add       BOOLEAN NOT NULL DEFAULT FALSE,
    monitor             BOOLEAN NOT NULL DEFAULT TRUE,
    monitor_type        TEXT NOT NULL DEFAULT 'all',
    quality_profile_id  TEXT NOT NULL DEFAULT '',
    library_id          TEXT NOT NULL DEFAULT '',
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

-- ── Import exclusions ────────────────────────────────────────────────────────

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
DROP TABLE IF EXISTS stats_snapshots;
DROP TABLE IF EXISTS activity_log;
DROP TABLE IF EXISTS episode_files;
DROP TABLE IF EXISTS media_management;
DROP TABLE IF EXISTS quality_definitions;
DROP TABLE IF EXISTS blocklist;
DROP TABLE IF EXISTS media_server_configs;
DROP TABLE IF EXISTS notification_configs;
DROP TABLE IF EXISTS download_client_configs;
DROP TABLE IF EXISTS grab_history;
DROP TABLE IF EXISTS indexer_configs;
DROP TABLE IF EXISTS episodes;
DROP TABLE IF EXISTS seasons;
DROP TABLE IF EXISTS series;
DROP TABLE IF EXISTS libraries;
DROP TABLE IF EXISTS quality_profiles;
