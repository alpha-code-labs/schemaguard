-- Canonical failure case #4 — ADD CHECK CONSTRAINT forcing a full
-- table scan during validation.
--
-- Adding a non-`NOT VALID` CHECK constraint forces Postgres to scan
-- every existing row and verify the predicate. While that scan
-- runs, Postgres holds an AccessExclusiveLock on the table,
-- blocking every concurrent read and write. On a large table the
-- lock duration scales with the row count — exactly the kind of
-- invisible outage the tool is designed to catch before it reaches
-- production.
--
-- The fix in production is to split this into two steps:
--   ALTER TABLE orders ADD CONSTRAINT ... CHECK (...) NOT VALID;
--   ALTER TABLE orders VALIDATE CONSTRAINT ...;
-- The first step takes a short lock; the second only blocks other
-- DDL. SchemaGuard surfaces the problem with the single-step form
-- so reviewers can propose the two-step pattern in the PR.
--
-- Expected SchemaGuard finding:
--   * Lock Risk: `[severity / lock or rewrite]` AccessExclusiveLock
--     on `public.orders`, held for the duration of the validation
--     scan. The severity depends on observed duration; on a 500k-row
--     table running a trivial `amount >= 0` check, the lock is
--     typically in the caution band — still obviously disruptive
--     for a production OLTP workload.
--   * Verdict: YELLOW or RED depending on measured duration.

ALTER TABLE orders
    ADD CONSTRAINT orders_amount_nonneg CHECK (amount >= 0);
