-- Indexes for the two hot read paths: competitor name search and the
-- registration roster scope. Both were sequential scans at every size.
--
-- Measured on PostgreSQL 18.4, stock configuration (shared_buffers 128MB,
-- work_mem 4MB, random_page_cost 4, max_connections 100), against synthetic
-- datasets shaped like production (5 events, 1.53 attendance rows per
-- competitor, current-event roster ~16% of all competitors).
-- Numbers are EXPLAIN (ANALYZE, BUFFERS) execution time, warm cache.
--
--   ?search= on 58,000 competitors (100x production)
--     before, three-way OR, seq scan .................  94.7 ms   'Smi'
--                                                      101.5 ms   'Gutierrez'
--     after, single expression + this GIN index ......   2.2 ms   'Smi'   (44x)
--                                                        2.5 ms   'Gutierrez' (40x)
--   ?search= on 5,800 competitors (10x production)
--     before .........................................   9.5 ms
--     after ..........................................   0.2 ms   (59x)
--
-- The index only works together with the predicate rewrite in
-- CompetitorService.GetAll. The old predicate was
--     name_first ILIKE ? OR name_last ILIKE ? OR CONCAT(name_first,' ',name_last) ILIKE ?
-- which no index can serve: CONCAT() is a function call rather than the ||
-- expression indexed here, so the third branch always forced a scan and the OR
-- dragged the other two down with it. Creating this index without that rewrite
-- buys nothing at all — it will simply never be chosen.
--
-- Search terms shorter than three characters cannot use a trigram index (there
-- is no whole trigram to look up) and still fall back to a sequential scan:
-- 43.6 ms for 'ar' at 58,000 rows, against 90.2 ms before the rewrite. That is
-- the floor for this design, and it is the one the check-in desk hits most,
-- since staff type two letters and read from the results. Fixing it needs a
-- different approach (prefix index, or a minimum search length in the UI), not
-- a different index.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- CREATE INDEX rather than CREATE INDEX CONCURRENTLY: the migration runner
-- wraps each file in a transaction, and CONCURRENTLY cannot run inside one.
-- This takes an ACCESS EXCLUSIVE lock on competitors for the duration of the
-- build, which at production size (580 rows) is milliseconds. If competitors
-- ever reaches a size where that lock matters, build it out of band instead.
CREATE INDEX IF NOT EXISTS idx_competitors_name_trgm
    ON competitors
    USING gin ((COALESCE(name_first, '') || ' ' || COALESCE(name_last, '')) gin_trgm_ops);

-- The registration list scopes every read to the current event with
--     EXISTS (SELECT 1 FROM competitor_events ce WHERE ce.competitor_id = ... AND ce.event_id = ?)
-- The existing unique index is on (competitor_id, event_id), so a lookup by
-- event_id alone cannot use it — event_id is not the leading column. Without
-- this index the EXISTS is a full scan of competitor_events that discards
-- roughly nine rows in ten.
--
--   registration list, 58,000 competitors / 88,476 attendance rows
--     before ...  27.4 ms  (Seq Scan on competitor_events, 78,830 rows discarded)
--     after ....  22.3 ms  (Bitmap Index Scan on this index)
--
-- An 18% improvement rather than an order of magnitude, because the dominant
-- cost is the unavoidable scan of competitors itself. It is included because it
-- is 624 kB at 100x production and the gap widens as attendance history
-- accumulates across events while any single roster stays one event's worth.
CREATE INDEX IF NOT EXISTS idx_competitor_events_event_id
    ON competitor_events (event_id);
