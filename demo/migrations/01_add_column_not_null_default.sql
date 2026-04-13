-- Canonical failure case #1 — ADD COLUMN NOT NULL DEFAULT.
--
-- Postgres 11+ optimizes `ADD COLUMN NOT NULL DEFAULT <constant>` to
-- a metadata-only change via pg_attribute.atthasmissing, so a naive
-- "ADD COLUMN x INT NOT NULL DEFAULT 0" would finish in milliseconds
-- without rewriting the table. That is NOT a useful demo of a
-- rewrite, so this migration uses a VOLATILE default — `random()` —
-- which Postgres cannot defer. Every row must be rewritten with its
-- own computed value, forcing an AccessExclusiveLock on `orders`
-- that is held for the duration of the rewrite.
--
-- Expected SchemaGuard finding:
--   * Lock Risk: `[stop / rewrite]` on `public.orders` — the
--     lockanalyzer observed-behavior heuristic flags
--     AccessExclusiveLock covering ~100% of the statement window as
--     a probable full-table rewrite.
--   * Verdict: RED (stop).

ALTER TABLE orders
    ADD COLUMN score DOUBLE PRECISION NOT NULL DEFAULT random();
