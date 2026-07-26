-- 004: competitors.name_first and name_last become NOT NULL.
--
-- The Go model has always declared these as plain strings, so the API cannot
-- write a NULL. Nothing else was stopping one: the columns were created
-- nullable, and the import binaries and any hand-run SQL write the table
-- directly. A NULL name breaks the name-match key BulkImport dedupes on
-- (nameKey concatenates the two) and renders as an empty row in the UI.
--
-- This is the schema half of the rule only. NOT NULL cannot reject '' — the
-- application layer rejects blank and whitespace-only names on create and on
-- edit, because Postgres has no opinion about a name made of spaces.
--
-- Backfill first so the constraint can be added without a rewrite failing on
-- legacy rows. Both the production restore and the upgrade clone report zero
-- NULLs, so this is expected to be a no-op there.

UPDATE competitors SET name_first = '' WHERE name_first IS NULL;
UPDATE competitors SET name_last  = '' WHERE name_last  IS NULL;

ALTER TABLE competitors
    ALTER COLUMN name_first SET NOT NULL,
    ALTER COLUMN name_last  SET NOT NULL;
