-- +goose Up

-- Series alternate titles (e.g. "Star Wars: Andor" for series "Andor").
-- Populated from TMDB's /tv/{id}/alternative_titles endpoint when a
-- series is added or its metadata is refreshed. Used by the release-
-- title gate (parser.TitleMatchesAny) so indexer responses with
-- alternate-language or marketing names don't get incorrectly dropped.
--
-- JSON array of strings, default empty so existing rows just become
-- single-title series until refreshed.
ALTER TABLE series
    ADD COLUMN alternate_titles JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE series DROP COLUMN alternate_titles;
