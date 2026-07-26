-- Contract phase. Everything dropped here is either derivable from
-- competitor_events / dob_verified_at or was already dead.
--
-- IMPORTANT: this is the one migration in the series that loses data. Deploy the
-- code that stops writing these columns FIRST, then this. Running it while the
-- previous release is still serving will break check-in for the rollout window,
-- because that code still writes last_registered_event.

-- Superseded by dob_verified_at (migration 002). The pair was one bit stored in
-- two columns, and requires_validation was always the exact inverse of validated.
ALTER TABLE competitors
    DROP COLUMN IF EXISTS requires_validation,
    DROP COLUMN IF EXISTS validated;

-- Superseded by competitor_events. A single text column cannot express
-- registration for two events at once, which is what forced a full CSV re-import
-- at the start of every event.
ALTER TABLE competitors
    DROP COLUMN IF EXISTS last_registered_event;

-- Dead since check-in state moved to competitor_events; the Go model stopped
-- referencing these long ago, but AutoMigrate never drops columns.
ALTER TABLE competitors
    DROP COLUMN IF EXISTS is_checked_in,
    DROP COLUMN IF EXISTS check_in_date_time,
    DROP COLUMN IF EXISTS checked_in_by;

-- Snapshot tables written by BulkImport before each import. They accumulate
-- forever and predate the uuid conversion, so they cannot be restored into the
-- current schema without manual work. Keep the most recent snapshot as a safety
-- net and drop the rest; going forward BulkImport enforces the same retention.
DO $$
DECLARE
    keep_suffix text;
    victim      text;
BEGIN
    SELECT substring(table_name from '_backup_(\d+)$')
    INTO keep_suffix
    FROM information_schema.tables
    WHERE table_schema = 'public'
      AND table_name ~ '^competitors_backup_\d+$'
    ORDER BY (substring(table_name from '_backup_(\d+)$'))::bigint DESC
    LIMIT 1;

    FOR victim IN
        SELECT table_name
        FROM information_schema.tables
        WHERE table_schema = 'public'
          AND table_name ~ '^(competitors|competitor_events)_backup_\d+$'
          AND (keep_suffix IS NULL OR substring(table_name from '_backup_(\d+)$') <> keep_suffix)
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I', victim);
    END LOOP;
END $$;
