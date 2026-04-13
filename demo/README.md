# SchemaGuard Demo

A small, real e-commerce schema with four canonical failure migrations that exercise every core detection pillar in SchemaGuard. Use this demo to see what the tool actually catches on migrations that look innocent in code but misbehave against real data.

This directory is self-contained. Everything you need to run SchemaGuard end-to-end — schema, seed SQL, pre-built `.dump` fixture, top-query config, four migration files, and a one-command driver — lives under `demo/`. The GitHub Action is wired in `.github/workflows/schemaguard-demo.yml` at the repository root.

See the [main SchemaGuard README](../README.md) for the full project context and the [source-of-truth docs](../docs/) for the product spec.

## What each of the four migrations demonstrates

All four migrations run against the same seeded `orders` table (~1M rows, matching the M7 target in `docs/tasks.md` 7.1). Each is a single-statement SQL file under `demo/migrations/`.

| # | File | Operation | Core detection pillar | Observed verdict on modern laptop |
|---|---|---|---|---|
| 01 | `01_add_column_not_null_default.sql` | `ADD COLUMN score DOUBLE PRECISION NOT NULL DEFAULT random()` | Lock risk — full table rewrite (`KindTableRewrite`) on `public.orders`, plus cascading `AccessExclusiveLock` holds on every affected index | 🔴 Stop |
| 02 | `02_create_index_blocking.sql` | Multi-column expression `CREATE INDEX` (no `CONCURRENTLY`) | Lock risk — `ShareLock` on `public.orders` that blocks writes for the duration of the build | 🟡 Caution (1–2 s ShareLock hold on a 1M-row orders table) |
| 03 | `03_rename_column.sql` | `ALTER TABLE orders RENAME COLUMN customer_id TO user_id` | Query plan regression — `KindQueryBroken` on every top query that selects or filters by `customer_id` (both `orders_by_customer` and `recent_orders` in this demo) | 🔴 Stop |
| 04 | `04_add_check_constraint.sql` | `ADD CONSTRAINT orders_amount_nonneg CHECK (amount >= 0)` | Lock risk — `AccessExclusiveLock` held for the duration of the validation scan; the M3 observed-behavior rewrite heuristic (AccessExclusive + ≥100 ms + ≥80% coverage) classifies this as a probable full-table rewrite because it looks identical to a rewrite from outside the SQL statement | 🟡 Caution |

The verdicts for #2 and #4 depend on observed duration on the machine SchemaGuard is running on. The committed first-pass thresholds in `docs/DECISIONS.md` put longer lock holds in the stop band and shorter ones in caution or info. On a ~1M-row seed on modern hardware, migration 02 is consistently caution and migration 04 is consistently caution. On slower CI hardware either may cross into the stop band; both are acceptable demonstrations of the lock-risk category.

### Why migration 01 uses `random()`

Postgres 11+ optimizes `ADD COLUMN NOT NULL DEFAULT <constant>` to a metadata-only change via `pg_attribute.atthasmissing`. A naive `DEFAULT 0` finishes in milliseconds without rewriting the table, which would *not* demonstrate a rewrite. Using a volatile default (`random()`) forces Postgres to compute a new value for every row and write it back — producing exactly the multi-second `AccessExclusive` hold that real rewrites cause in production.

## Prerequisites

- **Docker** — SchemaGuard v1 is Docker-only for the shadow database (see `docs/DECISIONS.md`). The Docker daemon must be running.
- **Go 1.25+** — for building the `schemaguard` CLI on your machine. The demo driver script builds it automatically the first time.

## Run the demo locally (one command)

```bash
bash demo/run-demo.sh
```

That's it. The script builds `schemaguard` if it does not already exist at the repo root, then runs `schemaguard check` against all four canonical migrations using the committed `demo/snapshot/demo.dump` and `demo/schemaguard.yaml`. Each migration produces a full text-format report — verdict, per-group findings, footer — in under ~20 seconds of tool-runtime on a modern laptop after the first `postgres:16-alpine` image pull.

To run a single migration:

```bash
bash demo/run-demo.sh 01
bash demo/run-demo.sh 03
```

Or pick a subset:

```bash
bash demo/run-demo.sh 01 02
```

Each run stops cleanly, tears down the shadow-DB Docker container, and leaves no residue.

## Regenerate the `.dump` fixture

The committed `demo/snapshot/demo.dump` is a Postgres custom-format dump produced from `demo/snapshot/seed.sql` via `pg_dump -Fc`. To regenerate it from scratch (for example, after editing `seed.sql`):

```bash
bash demo/snapshot/build.sh
```

The script spins up a temporary `postgres:16-alpine` container, runs the seed SQL, `pg_dump`s the result in custom format, and stops the container. The resulting `.dump` is ~8 MB — well within GitHub's 100 MB per-file limit — and restores via `pg_restore` significantly faster than piping a plain SQL file through `psql`, which is what keeps the demo-path under the committed 60-second target.

## How the CI is wired

The GitHub workflow at `.github/workflows/schemaguard-demo.yml` runs on `pull_request` events that touch `demo/**`, `action.yml`, `action/**`, `internal/**`, or `cmd/**`, and can also be triggered manually via `workflow_dispatch` to exercise any of the four canonical migrations.

The workflow uses the local `./` action, which is the `action.yml` committed at the repository root (the Milestone 6 deliverable). It passes three inputs:

```yaml
- uses: ./
  with:
    migration:     demo/migrations/<N>.sql   # selected by the Select step
    snapshot-path: demo/snapshot/demo.dump    # committed fixture
    config:        demo/schemaguard.yaml
```

On a `pull_request` event, the workflow uses `git diff --name-only` against the base branch to detect which canonical migration file under `demo/migrations/` was changed. If exactly one migration was modified, the Action runs against **that file** — so a PR that changes `03_rename_column.sql` gets a migration-03 comment, not a migration-01 comment. This directly satisfies the launch plan's requirement for "pre-made migration PRs, each triggering a distinct failure."

If no demo migration file was touched (e.g. the PR only changes `internal/` or `action/`), the SchemaGuard run is skipped cleanly so no misleading comment is posted. If multiple migration files were changed in one PR, the workflow fails with an explicit message asking for separate PRs — this preserves the single-comment-per-PR model without picking an arbitrary winner.

The `workflow_dispatch` path is unchanged: it lets you manually exercise any of the four migrations from the GitHub Actions UI without opening a PR.

On a PR, the Action builds the SchemaGuard CLI on the runner, invokes `schemaguard check --format markdown --out <tmp>`, logs the full rendered report to the Action log, and upserts a Markdown comment on the PR using the stable `<!-- schemaguard-comment: v1 -->` marker so subsequent commits update the same comment rather than creating duplicates.

## Contents

```
demo/
├── README.md                                (this file)
├── schemaguard.yaml                         (top queries + default thresholds)
├── run-demo.sh                              (one-command local driver)
├── snapshot/
│   ├── seed.sql                             (schema + ~1M rows via generate_series)
│   ├── build.sh                             (regenerates demo.dump from seed.sql)
│   └── demo.dump                            (committed pg_dump custom-format fixture)
└── migrations/
    ├── 01_add_column_not_null_default.sql   (volatile default → table rewrite)
    ├── 02_create_index_blocking.sql         (non-CONCURRENTLY index → ShareLock)
    ├── 03_rename_column.sql                 (rename column → broken top query)
    └── 04_add_check_constraint.sql          (CHECK validation → table scan lock)
```

## What the demo does NOT include

Per the committed scope in `docs/tasks.md` Milestone 7, the demo intentionally omits:

- **The dbt fifth case.** A fifth migration dropping a column referenced by a dbt model is the Milestone 9 deliverable and only ships if the secondary dbt capability goes live in v1 — see `docs/requirements.md` Success Criteria item 2.
- **Cloud-storage snapshot sources.** Only the committed M6 snapshot sources (`snapshot-path` and `snapshot-url`) are supported; S3/GCS/Azure are deferred to v1.5.
- **Per-query threshold overrides.** The committed M4 defaults are deliberately used as-is so the demo showcases the out-of-the-box behavior.

These are tracked in [`docs/things_to_fix.md`](../docs/things_to_fix.md).
