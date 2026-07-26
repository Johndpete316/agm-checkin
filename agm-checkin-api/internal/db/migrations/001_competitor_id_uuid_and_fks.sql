-- 001: make competitor_id a real uuid and add the missing foreign keys.
--
-- competitors.id is uuid but competitor_events.competitor_id and
-- competitor_schedules.competitor_id were created as text, so the foreign keys
-- could never be added. Without them, deleting a competitor silently orphans
-- their attendance history. This cleans up the rows that gap already produced,
-- converts the column type, then adds the constraints.
--
-- Must work against two starting states: an existing database where the column
-- is still text, and a freshly AutoMigrated one where the model tag has already
-- made it uuid. Comparisons before the conversion therefore cast both sides to
-- text, and everything after it can assume uuid.

-- Attendance rows whose competitor no longer exists.
DELETE FROM competitor_events ce
WHERE NOT EXISTS (
    SELECT 1 FROM competitors c WHERE c.id::text = ce.competitor_id::text);

DELETE FROM competitor_schedules cs
WHERE NOT EXISTS (
    SELECT 1 FROM competitors c WHERE c.id::text = cs.competitor_id::text);

-- Rows pointing at an event that no longer exists.
DELETE FROM competitor_events ce
WHERE NOT EXISTS (SELECT 1 FROM events e WHERE e.id = ce.event_id);

DELETE FROM competitor_schedules cs
WHERE NOT EXISTS (SELECT 1 FROM events e WHERE e.id = cs.event_id);

-- Every remaining competitor_id now equals some competitors.id, so the cast is
-- safe. On an already-uuid column this is a no-op.
ALTER TABLE competitor_events    ALTER COLUMN competitor_id TYPE uuid USING competitor_id::uuid;
ALTER TABLE competitor_schedules ALTER COLUMN competitor_id TYPE uuid USING competitor_id::uuid;

-- The inverse gap: a competitor whose last_registered_event names an event they
-- have no attendance row for. These are real registrations, so restore the
-- missing row rather than dropping the claim. Once competitor_events becomes the
-- single source of truth these competitors would otherwise vanish from the
-- registration view.
--
-- Guarded on the column existing: the second starting state this migration has
-- to support is a freshly AutoMigrated database, where the current Go model has
-- already omitted last_registered_event, so the column is never created and an
-- unguarded reference is a parse-time failure rather than a no-op. There is
-- nothing to backfill in that state -- a fresh database has no legacy claims.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name   = 'competitors'
          AND column_name  = 'last_registered_event')
    THEN
        INSERT INTO competitor_events (id, competitor_id, event_id, checked_in)
        SELECT gen_random_uuid(), c.id, c.last_registered_event, false
        FROM competitors c
        WHERE COALESCE(c.last_registered_event, '') <> ''
          AND EXISTS (SELECT 1 FROM events e WHERE e.id = c.last_registered_event)
          AND NOT EXISTS (
                SELECT 1 FROM competitor_events ce
                WHERE ce.competitor_id = c.id
                  AND ce.event_id = c.last_registered_event);
    END IF;
END $$;

-- CASCADE on the competitor side: deleting a competitor should take their
-- attendance history with it. RESTRICT on the event side: an event with
-- registrations must not be deletable.
ALTER TABLE competitor_events
    ADD CONSTRAINT fk_competitor_events_competitor
    FOREIGN KEY (competitor_id) REFERENCES competitors (id) ON DELETE CASCADE;

ALTER TABLE competitor_events
    ADD CONSTRAINT fk_competitor_events_event
    FOREIGN KEY (event_id) REFERENCES events (id) ON DELETE RESTRICT;

ALTER TABLE competitor_schedules
    ADD CONSTRAINT fk_competitor_schedules_competitor
    FOREIGN KEY (competitor_id) REFERENCES competitors (id) ON DELETE CASCADE;

ALTER TABLE competitor_schedules
    ADD CONSTRAINT fk_competitor_schedules_event
    FOREIGN KEY (event_id) REFERENCES events (id) ON DELETE RESTRICT;
