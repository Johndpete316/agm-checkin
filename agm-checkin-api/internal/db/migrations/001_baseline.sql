-- 001: baseline — the schema the Go models cannot express.
--
-- Squashes the original 001_competitor_id_uuid_and_fks, 002_dob_verified and
-- 003_drop_legacy_columns. Those were one-time repairs of a database shape that
-- no longer exists: production is past all three, and a database created fresh
-- from internal/db/db.go is born without the problems they fixed (competitor_id
-- already carries type:uuid, dob_verified_at is a model field, and the legacy
-- columns are simply never created).
--
-- Replaying them was in fact impossible. They referenced last_registered_event,
-- validated and requires_validation — columns the models had already dropped —
-- so the old 001 failed with SQLSTATE 42703 on any new database, taking API
-- startup down with it. Squashing is what makes a fresh install work at all.
--
-- What survives is only the part AutoMigrate genuinely cannot produce:
--   * foreign keys — the models carry no relation fields or foreignKey tags, so
--     AutoMigrate emits none. Without them, deleting a competitor silently
--     orphans their attendance and schedule rows.
--   * a partial index — not expressible as a struct tag.
--
-- Idempotent across both states it can actually meet: a fresh AutoMigrate'd
-- database where none of this exists, and production where all of it does.
-- Against a database in neither state (one still holding competitor_id as text,
-- say) the ALTER fails loudly rather than half-applying — the transaction in
-- applyOne rolls back and nothing is recorded, which is the intended behaviour.

-- CASCADE on the competitor side: deleting a competitor takes their attendance
-- history with it. RESTRICT on the event side: an event with registrations must
-- not be deletable. Postgres has no ADD CONSTRAINT IF NOT EXISTS for foreign
-- keys, so each one is guarded by a catalogue check instead.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_competitor_events_competitor'
          AND conrelid = 'competitor_events'::regclass
    ) THEN
        ALTER TABLE competitor_events
            ADD CONSTRAINT fk_competitor_events_competitor
            FOREIGN KEY (competitor_id) REFERENCES competitors (id) ON DELETE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_competitor_events_event'
          AND conrelid = 'competitor_events'::regclass
    ) THEN
        ALTER TABLE competitor_events
            ADD CONSTRAINT fk_competitor_events_event
            FOREIGN KEY (event_id) REFERENCES events (id) ON DELETE RESTRICT;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_competitor_schedules_competitor'
          AND conrelid = 'competitor_schedules'::regclass
    ) THEN
        ALTER TABLE competitor_schedules
            ADD CONSTRAINT fk_competitor_schedules_competitor
            FOREIGN KEY (competitor_id) REFERENCES competitors (id) ON DELETE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_competitor_schedules_event'
          AND conrelid = 'competitor_schedules'::regclass
    ) THEN
        ALTER TABLE competitor_schedules
            ADD CONSTRAINT fk_competitor_schedules_event
            FOREIGN KEY (event_id) REFERENCES events (id) ON DELETE RESTRICT;
    END IF;
END $$;

-- Carried over from the old 002. The only question ever asked of this column is
-- "who still needs verifying", and that set stays small as the roster grows.
CREATE INDEX IF NOT EXISTS idx_competitors_dob_unverified
    ON competitors (id) WHERE dob_verified_at IS NULL;
