-- Canonical failure case #2 — CREATE INDEX without CONCURRENTLY.
--
-- A plain `CREATE INDEX` holds a ShareLock on the table for the
-- entire duration of the index build. That mode blocks concurrent
-- INSERT / UPDATE / DELETE but allows SELECT — the classic "writes
-- stop during the build" regression that teams hit when they forget
-- to use CREATE INDEX CONCURRENTLY in production.
--
-- The fix in production code is `CREATE INDEX CONCURRENTLY`, which
-- does not take a ShareLock. SchemaGuard does NOT suggest the fix —
-- it is a detection tool, not a refactoring tool — it just flags the
-- blocking behavior observed on the shadow database.
--
-- This migration deliberately builds a five-key expression index so
-- the build is slow enough on the ~1M-row demo orders table to push
-- the observed lock duration past the committed 1-second caution
-- threshold for ShareLock from docs/DECISIONS.md. A single-column
-- index on the same table would finish in under a second on modern
-- SSD-backed hardware and produce only an info-severity finding,
-- which would not demonstrate the lock-risk category.
--
-- Expected SchemaGuard finding:
--   * Lock Risk: `[caution / lock]` ShareLock on `public.orders`
--     with `blocks writes` in the Impact column. Severity lands in
--     the caution band (>1s) per the committed thresholds. On
--     slower CI hardware the hold may cross into the stop band; both
--     are acceptable demonstrations of the ShareLock write-blocking
--     behavior.
--   * Verdict: YELLOW or RED depending on observed duration.

CREATE INDEX idx_orders_write_blocker ON orders (
    customer_id,
    status,
    amount,
    created_at,
    (customer_id * 31 + (amount * 100)::bigint)
);
