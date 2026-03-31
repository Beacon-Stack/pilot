-- +goose Up

-- ── Download clients ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS download_client_configs (
    id         TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL,
    kind       TEXT    NOT NULL,             -- "qbittorrent", "transmission", etc.
    enabled    INTEGER NOT NULL DEFAULT 1,
    priority   INTEGER NOT NULL DEFAULT 25,
    settings   TEXT    NOT NULL DEFAULT '{}', -- JSON: url, username, password, etc.
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

-- ── Notifications ────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS notification_configs (
    id         TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL,
    kind       TEXT    NOT NULL,             -- "webhook", "discord", "email"
    enabled    INTEGER NOT NULL DEFAULT 1,
    settings   TEXT    NOT NULL DEFAULT '{}', -- JSON: plugin-specific settings
    on_events  TEXT    NOT NULL DEFAULT '[]', -- JSON array of event type strings
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

-- ── Media servers ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS media_server_configs (
    id         TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL,
    kind       TEXT    NOT NULL,             -- "plex", "emby", "jellyfin"
    enabled    INTEGER NOT NULL DEFAULT 1,
    settings   TEXT    NOT NULL DEFAULT '{}', -- JSON: plugin-specific settings
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

-- ── Blocklist ─────────────────────────────────────────────────────────────────
-- Adapted from Screenarr: uses series_id + episode_id instead of movie_id.

CREATE TABLE IF NOT EXISTS blocklist (
    id            TEXT     PRIMARY KEY,
    series_id     TEXT     NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    episode_id    TEXT     REFERENCES episodes(id) ON DELETE SET NULL,
    release_guid  TEXT     NOT NULL,
    release_title TEXT     NOT NULL,
    indexer_id    TEXT,
    protocol      TEXT     NOT NULL DEFAULT '',
    size          INTEGER  NOT NULL DEFAULT 0,
    added_at      DATETIME NOT NULL,
    notes         TEXT     NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_blocklist_series_id  ON blocklist(series_id);
CREATE INDEX IF NOT EXISTS idx_blocklist_episode_id ON blocklist(episode_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_blocklist_guid ON blocklist(release_guid);

-- ── Quality definitions ───────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS quality_definitions (
    id             TEXT    PRIMARY KEY, -- stable slug: "<resolution>-<source>-<codec>-<hdr>"
    name           TEXT    NOT NULL,    -- human-readable label, e.g. "1080p Bluray"
    resolution     TEXT    NOT NULL,
    source         TEXT    NOT NULL,
    codec          TEXT    NOT NULL,
    hdr            TEXT    NOT NULL,
    min_size       REAL    NOT NULL DEFAULT 0,   -- MB per minute of runtime (0 = no minimum)
    max_size       REAL    NOT NULL DEFAULT 0,   -- MB per minute of runtime (0 = no limit)
    preferred_size REAL    NOT NULL DEFAULT 0,   -- MB per minute target (0 = same as max)
    sort_order     INTEGER NOT NULL DEFAULT 0
);

-- Seed with the 14 standard quality levels (lowest → highest).
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

-- ── Media management ──────────────────────────────────────────────────────────
-- Adapted for TV: episode naming formats, season folder format, series folder.

CREATE TABLE IF NOT EXISTS media_management (
    id                        INTEGER PRIMARY KEY CHECK (id = 1),
    rename_episodes           INTEGER NOT NULL DEFAULT 1,
    standard_episode_format   TEXT    NOT NULL DEFAULT '{Series Title} - S{season:00}E{episode:00} - {Episode Title} {Quality Full}',
    daily_episode_format      TEXT    NOT NULL DEFAULT '{Series Title} - {Air-Date} - {Episode Title} {Quality Full}',
    anime_episode_format      TEXT    NOT NULL DEFAULT '{Series Title} - S{season:00}E{episode:00} - {Episode Title} {Quality Full}',
    series_folder_format      TEXT    NOT NULL DEFAULT '{Series Title} ({Year})',
    season_folder_format      TEXT    NOT NULL DEFAULT 'Season {season:00}',
    colon_replacement         TEXT    NOT NULL DEFAULT 'space-dash',
    import_extra_files        INTEGER NOT NULL DEFAULT 0,
    extra_file_extensions     TEXT    NOT NULL DEFAULT 'srt,nfo',
    unmonitor_deleted_episodes INTEGER NOT NULL DEFAULT 0
);

INSERT INTO media_management (id) VALUES (1);

-- +goose Down

DROP TABLE IF EXISTS media_management;
DROP TABLE IF EXISTS quality_definitions;
DROP TABLE IF EXISTS blocklist;
DROP TABLE IF EXISTS media_server_configs;
DROP TABLE IF EXISTS notification_configs;
DROP TABLE IF EXISTS download_client_configs;
