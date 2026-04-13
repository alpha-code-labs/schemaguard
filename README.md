# SchemaGuard

Catch unsafe Postgres migrations in CI — by actually running them.

> Static linters check what your SQL looks like. SchemaGuard checks what your migration actually **does** — lock durations, query-plan regressions, broken downstream queries — by running it against a shadow copy of your real data and posting the verdict as a PR comment.

## Install

```bash
go install github.com/schemaguard/schemaguard/cmd/schemaguard@latest
```

Requires Go 1.25+ and a running Docker daemon.

## 30-second quickstart

```bash
# 1. Install
go install github.com/schemaguard/schemaguard/cmd/schemaguard@latest

# 2. Clone the demo
git clone https://github.com/schemaguard/schemaguard.git
cd schemaguard

# 3. Run against a canonical failure migration
schemaguard check \
  --migration demo/migrations/01_add_column_not_null_default.sql \
  --snapshot  demo/snapshot/demo.dump \
  --config    demo/schemaguard.yaml
```

The tool stands up a Docker-based ephemeral Postgres, restores the snapshot, applies the migration inside a single transaction, samples `pg_locks` in parallel, captures `EXPLAIN (FORMAT JSON)` plans before and after, and prints a red/yellow/green verdict with grouped findings — all in under 30 seconds.

See [`demo/README.md`](./demo/README.md) for the full walkthrough of all four canonical failure cases.

## How it works

SchemaGuard is a **dynamic verification** tool, not a static linter. It complements tools like [Squawk](https://github.com/sbdchd/squawk) (static SQL linting) and [Atlas](https://atlasgo.io/) (migration lint + schema management) — use all three if you want full coverage.

| Layer | What it checks | SchemaGuard's role |
|---|---|---|
| Static lint (Squawk, Atlas) | SQL syntax, known-bad patterns | Complementary — use alongside |
| **Dynamic verification (SchemaGuard)** | **Real lock durations, query-plan regressions, broken queries** | **This tool** |
| Data diff (Datafold) | Value-level changes in analytics warehouse | Different layer — not overlapping |

SchemaGuard provisions a Docker-based shadow Postgres, restores your masked production snapshot, executes the migration, and measures what actually happens. It does not guess from SQL patterns — it observes real behavior.

## What it catches

- **Lock risk** — `AccessExclusiveLock` held for seconds during a table rewrite, `ShareLock` blocking writes during a non-concurrent index build, full-table-scan CHECK constraint validation
- **Query-plan regressions** — a dropped index causing `Index Scan → Seq Scan`, a renamed column breaking a top query, estimated-cost explosions
- **Migration failures** — a migration that errors mid-flight is caught, the transaction is rolled back, pre-failure lock findings are preserved, and post-migration plan capture is skipped

## Supported today

- Postgres only (MySQL, SQL Server, Snowflake are not supported)
- Docker-only shadow DB (no external-Postgres mode yet)
- CLI + GitHub Action (no dashboard, no SaaS)
- Plan-only `EXPLAIN (FORMAT JSON)` — never `EXPLAIN ANALYZE`
- Lock severity thresholds are committed first-pass defaults, not yet user-tunable
- Query-plan regression thresholds are overridable via `schemaguard.yaml`

## Not yet supported

- dbt downstream impact detection (deferred to v1.5)
- Automatic top-query discovery from `pg_stat_statements`
- Cloud-storage snapshot sources (S3, GCS, Azure)
- Per-query threshold overrides
- Multi-database support
- Web dashboard, SaaS, or hosted mode

## Usage

```
schemaguard check --help
```

### Required flags

```
--migration <path>    Path to the migration SQL file or directory
--snapshot  <path>    Path to a Postgres dump file (.sql, .dump, or .tar)
```

### Optional flags

```
--config  <path>      YAML config with top queries and threshold overrides
--format  <fmt>       Output format: text (default), json, or markdown
--out     <path>      Write the report to a file instead of stdout
```

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Green — no significant findings |
| `1` | Yellow — caution-level findings, merge with care |
| `2` | Red — stop-level findings, do not merge |
| `3` | Tool error — bad inputs, Docker unavailable, or crash |

## GitHub Action

```yaml
- uses: schemaguard/schemaguard@v0.1.0
  with:
    migration:     path/to/migration.sql
    snapshot-path: path/to/snapshot.dump
    config:        path/to/schemaguard.yaml   # optional
```

The Action builds the CLI on the runner, executes the check, posts (or updates) a Markdown PR comment with the verdict and findings, and sets the step status based on the exit code. See [`action.yml`](./action.yml) for the full input/output schema.

## Documentation

- [`docs/requirements.md`](./docs/requirements.md) — product truth
- [`docs/build_spec.md`](./docs/build_spec.md) — implementation truth
- [`docs/tasks.md`](./docs/tasks.md) — execution order and milestone definitions
- [`docs/DECISIONS.md`](./docs/DECISIONS.md) — settled decisions

## Contributing

Contributions welcome. Please open an issue before submitting a PR so we can discuss scope.

## License

MIT — see [`LICENSE`](./LICENSE).
