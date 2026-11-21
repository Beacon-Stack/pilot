-- +goose Up

-- TheTVDB's id for the series, from TMDB's external_ids. Most torrent
-- and usenet indexers match a show by it (?t=tvsearch&tvdbid=...).
-- NULL: not looked up yet. 0: TMDB has none for this series.
ALTER TABLE series ADD COLUMN tvdb_id INTEGER;

-- +goose Down
ALTER TABLE series DROP COLUMN tvdb_id;
