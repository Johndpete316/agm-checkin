-- 002: collapse requires_validation + validated into one nullable timestamp.
--
-- The two booleans were written as exact logical inverses by the importer
-- (bin/import/main.go) and with unrelated semantics by the seeder, so together
-- they encoded one bit across two columns plus two unreachable states. The real
-- rule is simpler: a competitor's date of birth is either verified or it isn't,
-- and once verified it stays verified. That is one nullable timestamp.
--
-- dob_verified_by records who did it. Provenance comes from audit_logs where we
-- have it; everything verified before audit logging existed is attributed to the
-- historical import.
--
-- The old columns are deliberately left in place and still written by the API,
-- so this phase can be rolled back without losing state. Migration 004 drops them.

ALTER TABLE competitors
    ADD COLUMN IF NOT EXISTS dob_verified_at timestamptz,
    ADD COLUMN IF NOT EXISTS dob_verified_by text NOT NULL DEFAULT '';

-- Backfill and re-derive, both guarded on the legacy columns still existing.
-- This migration has to apply to a freshly AutoMigrated database as well as to
-- an upgrading one, and the current Go model no longer declares validated or
-- requires_validation, so on a fresh database those columns are never created.
-- An unguarded reference fails the whole migration and takes API startup with
-- it. A fresh database has nothing to backfill and no legacy encoding to keep in
-- sync, so skipping both statements is the correct behaviour, not a compromise.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name   = 'competitors'
          AND column_name  = 'validated')
    THEN
        RETURN;
    END IF;

    -- Backfill. A competitor counts as verified only if they were marked
    -- validated AND we actually hold a date of birth for them — claiming we
    -- verified a date we do not have is worse than asking for ID once more at
    -- the desk. Go's zero time is the "no DOB on file" sentinel here, not NULL.
    WITH verified AS (
        SELECT c.id,
               COALESCE(a.created_at, TIMESTAMPTZ '2026-04-13 00:00:00+00')   AS verified_at,
               COALESCE(NULLIF(a.actor_name, ''), 'historical import')        AS verified_by
        FROM competitors c
        LEFT JOIN LATERAL (
            SELECT al.created_at, al.actor_name
            FROM audit_logs al
            WHERE al.action = 'competitor.validated'
              AND al.entity_id = c.id::text
            ORDER BY al.created_at
            LIMIT 1
        ) a ON true
        WHERE c.validated
          AND c.date_of_birth IS NOT NULL
          AND c.date_of_birth <> TIMESTAMPTZ '0001-01-01 00:00:00+00'
    )
    UPDATE competitors c
    SET dob_verified_at = v.verified_at,
        dob_verified_by = v.verified_by
    FROM verified v
    WHERE c.id = v.id;

    -- Re-derive the legacy booleans from the new field so the two encodings
    -- agree everywhere. Without this the 57 rows that were (true, true), the one
    -- corrupt (false, false) row, and the 10 we just un-verified would keep
    -- contradicting dob_verified_at, and a rollback would resurrect the old
    -- policy on exactly the rows the new one was meant to fix. The API writes
    -- both columns the same way, so this invariant holds from here until
    -- migration 003 drops them.
    UPDATE competitors
    SET validated           = (dob_verified_at IS NOT NULL),
        requires_validation = (dob_verified_at IS NULL)
    WHERE validated           <> (dob_verified_at IS NOT NULL)
       OR requires_validation <> (dob_verified_at IS NULL);
END $$;

-- Partial index: the only question ever asked of these columns is "who still
-- needs verifying", and that set stays small as the roster grows.
CREATE INDEX IF NOT EXISTS idx_competitors_dob_unverified
    ON competitors (id) WHERE dob_verified_at IS NULL;
