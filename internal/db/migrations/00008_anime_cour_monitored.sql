-- +goose Up

-- Per-cour monitoring overrides for anime series.
--
-- Pilot displays anime series with their TMDB layout (one big season for
-- multi-cour shows like Jujutsu Kaisen — TMDB serves all 59 episodes as
-- "Season 1"). The UI projects a cour-shaped view at read time using the
-- Anime-Lists XML mapping data (loaded in-memory by the animelist
-- service). To let users monitor a specific cour (e.g. just the latest
-- one for ongoing shows) without changing the underlying TMDB-shape
-- episodes table, monitor state is stored here keyed by tvdb_season —
-- the same coordinate Anime-Lists XML uses.
--
-- A row exists only when the user has explicitly toggled monitoring;
-- the absence of a row means "inherit the parent season's monitored
-- bit," so default behavior matches non-anime series.
CREATE TABLE anime_cour_monitored (
    series_id    TEXT    NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    tvdb_season  INTEGER NOT NULL,
    monitored    BOOLEAN NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (series_id, tvdb_season)
);

-- +goose Down
DROP TABLE anime_cour_monitored;
