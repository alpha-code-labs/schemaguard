-- SchemaGuard demo seed.
--
-- Creates a small but realistic e-commerce schema and seeds it with
-- roughly 1M rows across `orders` and `order_items` so lock durations
-- and query-plan costs are observable when the canonical failure
-- migrations run. Row counts are sized to stay comfortably under the
-- demo-path 60-second run target on a modern laptop (see
-- docs/DECISIONS.md and docs/tasks.md 7.4).
--
-- This file is the single source of truth for the demo snapshot. The
-- committed demo.dump fixture in this directory is generated from
-- this file via build.sh.

CREATE TABLE users (
    id          BIGSERIAL PRIMARY KEY,
    email       TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE products (
    id           BIGSERIAL PRIMARY KEY,
    sku          TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    price_cents  INTEGER NOT NULL
);

CREATE TABLE orders (
    id           BIGSERIAL PRIMARY KEY,
    customer_id  BIGINT NOT NULL,
    amount       NUMERIC(10,2) NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE order_items (
    id                BIGSERIAL PRIMARY KEY,
    order_id          BIGINT NOT NULL,
    product_id        BIGINT NOT NULL,
    quantity          INTEGER NOT NULL,
    unit_price_cents  INTEGER NOT NULL
);

-- 10,000 users — enough variety for customer_id lookups.
INSERT INTO users (email)
SELECT 'user' || i || '@example.com'
FROM generate_series(1, 10000) AS i;

-- 500 products — small catalog.
INSERT INTO products (sku, name, price_cents)
SELECT
    'SKU-' || lpad(i::text, 6, '0'),
    'Product ' || i,
    100 + ((i * 31) % 10000)
FROM generate_series(1, 500) AS i;

-- 1,000,000 orders — the main table the canonical migrations
-- exercise. Matches the "~1M rows in orders" target from
-- docs/tasks.md 7.1. 1M rows is large enough that the canonical
-- CREATE INDEX migration crosses the committed 1-second caution
-- threshold for ShareLock while staying comfortably under the
-- 60-second demo-path target for the slower ALTER TABLE rewrite.
INSERT INTO orders (customer_id, amount, status)
SELECT
    (i % 10000) + 1,
    ((i * 31) % 100000)::numeric / 100,
    CASE (i % 3)
        WHEN 0 THEN 'pending'
        WHEN 1 THEN 'shipped'
        ELSE        'delivered'
    END
FROM generate_series(1, 1000000) AS i;

-- 500,000 order_items — one per two orders on average. Kept smaller
-- than `orders` so the committed demo.dump fixture stays under
-- GitHub's 100 MB per-file limit with room to spare.
INSERT INTO order_items (order_id, product_id, quantity, unit_price_cents)
SELECT
    ((i - 1) % 1000000) + 1,
    (i % 500) + 1,
    (i % 5) + 1,
    100 + ((i * 13) % 10000)
FROM generate_series(1, 500000) AS i;

-- Indexes on orders so the baseline plans captured by
-- `schemaguard check` are interesting. The demo migrations expose
-- regressions against these.
CREATE INDEX idx_orders_customer   ON orders (customer_id);
CREATE INDEX idx_orders_status     ON orders (status);
CREATE INDEX idx_order_items_order ON order_items (order_id);

-- Fresh statistics so baseline EXPLAIN matches what a real production
-- Postgres would plan.
ANALYZE users;
ANALYZE products;
ANALYZE orders;
ANALYZE order_items;
