# Build Spec — SchemaGuard v1

## Purpose of this document

This document is the **implementation reference** for building SchemaGuard v1. It translates the product truths in `requirements.md` into a concrete build plan: what the system is made of, how the pieces fit together, what gets built first, what gets deferred, and what the quality bar is before launch.

It is not a product requirements document. It does not define scope, goals, non-goals, or success criteria — those live in `requirements.md` and are the source of truth. Where this document and `requirements.md` disagree, `requirements.md` wins.

This document is written for an engineer or AI coding agent who needs to build v1 end-to-end without re-deriving product decisions.

---

## Build goals for v1

1. Ship a single command that takes a Postgres migration and a masked snapshot and returns a clear risk report.
2. Ship a GitHub Action that wraps the CLI and posts the report as a PR comment.
3. Reliably detect the two core failure classes: **lock risk** and **query plan regression**.
4. Ship a public demo repository that exercises the canonical failure modes end-to-end.
5. Keep the codebase small, modular, and honest — no hidden abstractions, no premature generalization.

The secondary dbt impact capability is included in v1 **only if the core is stable and time remains**. It must never block shipping the core.

---

## High-level system shape

SchemaGuard v1 is a **single CLI binary** (or thin runtime) with an optional **GitHub Action wrapper**. No server, no persistent state, no background processes.

The CLI orchestrates a short, linear pipeline:

```
Inputs → Shadow DB Provision → Baseline Capture → Migration Execute (instrumented)
       → Post-Migration Capture → Analysis → Report → Output
```

Each stage is a self-contained module. Stages do not share mutable state — they hand results forward as plain data structures.

External dependencies v1 must tolerate:
- A local Docker daemon (for the ephemeral Postgres container)
- A Postgres dump file the CLI can restore into the shadow DB
- A migration SQL file or directory

External dependencies v1 must **not** require:
- Cloud accounts, cloud APIs, or cloud credentials
- A database other than Postgres
- A network connection beyond fetching the optional GitHub Action from the marketplace
- Any externally-provided Postgres instance — v1 always provisions its own shadow DB via Docker. Support for pointing at an already-running Postgres is deferred to v1.5.

---

## Core execution flow

The CLI executes the following sequence on every run. Each step must be independently testable.

1. **Parse inputs.** Read CLI flags and config file. Validate that required inputs exist and are readable. Fail fast with a clear error if anything is missing.
2. **Provision shadow DB.** Start a Docker-based ephemeral Postgres instance and restore the user-supplied snapshot into it. Record restore time and success.
3. **Capture baseline.** For each user-specified top query, run `EXPLAIN (FORMAT JSON)` against the shadow DB and store the plan. **Plan-only `EXPLAIN` — `EXPLAIN ANALYZE` is not used in v1, and the analyzer never executes the top queries.**
4. **Execute migration (instrumented).** Apply the migration SQL to the shadow DB **inside a single transaction by default**. If the migration file contains explicit `BEGIN` / `COMMIT` statements, those are respected instead of being wrapped. Sample lock state and wall-clock timing concurrently with execution. **On the first SQL error, stop applying further statements, mark the run as failed, and preserve any lock findings already produced for statements that executed. Do not continue past an error.**
5. **Capture post-migration state.** Re-run `EXPLAIN (FORMAT JSON)` on the same top queries against the now-migrated shadow DB and record any query that now errors. **This step is skipped entirely if step 4 halted due to a migration error** — in that case no post-migration plans exist and the query plan regression analyzer produces no findings for this run.
6. **Analyze.** Feed captured data into the lock analyzer and the query plan regression analyzer. Each produces a list of findings with severity and a short explanation. If step 5 was skipped, only lock findings and a migration-failure finding are available.
7. **(Optional) dbt impact analysis.** If a dbt `manifest.json` is provided and the secondary capability is enabled, parse the manifest, intersect affected tables/columns with model references, and produce dbt-specific findings.
8. **Generate report.** Combine all findings into a structured report object (severity-ranked, grouped).
9. **Emit output.** Write human-readable output to stdout, optional JSON to a file, and (in Action mode) a Markdown PR comment.
10. **Tear down.** Stop and remove the ephemeral Postgres instance. Clean up any temporary files.
11. **Exit with severity-based code.**

The full pipeline must be **idempotent and stateless**. Running the tool twice on the same inputs should produce equivalent reports.

---

## Primary components

The codebase should be organized around these components. Each owns one responsibility, exposes a narrow interface, and can be replaced or tested in isolation.

### 1. CLI entry point

The top-level command. Parses flags, loads config, invokes the orchestrator, handles signals (Ctrl-C must cleanly tear down the shadow DB), and selects the correct output format.

### 2. Shadow DB runner

Responsible for starting a **Docker-based** ephemeral Postgres instance, restoring a snapshot into it, and tearing it down reliably. Must clean up even on crash, panic, or forced termination. Support for pointing at an externally-managed Postgres instance is explicitly deferred to v1.5 — v1 always provisions its own ephemeral Docker container.

### 3. Migration executor

Applies the migration SQL against the shadow DB, statement by statement, with instrumentation hooks for the lock analyzer. Records per-statement wall-clock time, SQL errors, and completion status. Does not interpret SQL semantically — treats statements as opaque units to execute and measure.

**Transaction semantics.** The executor wraps the full migration file in a single transaction by default. If the migration file itself contains explicit `BEGIN` / `COMMIT` statements, the executor respects those instead of wrapping. This matches how Flyway and Alembic execute Postgres migrations in practice and produces a coherent post-state for downstream analysis.

**Failure behavior.** On the first SQL error, the executor stops applying further statements, marks the run as failed, and returns a structured failure result. Lock findings already produced for the statements that did execute are preserved and surfaced in the final report alongside a red migration-failure finding. Post-migration plan capture is skipped entirely on a failed run. There is no "continue past the error" path in v1.

### 4. Lock analyzer

Runs in parallel with the migration executor. Samples `pg_locks` and `pg_stat_activity` at a configurable interval, attributes observed locks to the statement currently executing, and produces findings: lock type, affected object, duration, whether the lock blocks reads or writes. Also recognizes statement patterns that imply full table rewrites (and flags them with measured data, not static rules).

### 5. Query plan regression analyzer

Takes the baseline and post-migration `EXPLAIN (FORMAT JSON)` plans for each top query and diffs them. **Uses plan-only `EXPLAIN` — `EXPLAIN ANALYZE` is explicitly not used in v1, and the analyzer never executes the top queries.** Produces findings for:

- scan type downgrades (e.g. Index Scan → Seq Scan)
- estimated cost increases beyond the configured threshold
- **estimated rows delta beyond threshold** (derived from plan estimates, not runtime measurement)
- newly introduced full-table scans
- queries that now error outright

The analyzer runs only when the migration executor completed successfully. If the executor halted on error, post-migration plans do not exist and this analyzer produces no findings for that run.

Diff logic should be small, deterministic, and explainable. No ML, no heuristics that cannot be reproduced from the raw plan JSON.

### 6. Report generator

Consumes the findings from the analyzers and produces three outputs:
- A human-readable text report for stdout
- A JSON document for machine consumption
- A Markdown report sized for a GitHub PR comment

All three are derived from the same underlying report data structure. The top-line verdict (🟢 / 🟡 / 🔴) is computed from the highest-severity finding in the set.

### 7. GitHub Action wrapper

A thin wrapper that:
- Checks out the PR branch
- Resolves the migration file(s) affected by the PR
- Resolves the snapshot location (the set of supported snapshot sources for v1 is closed during Milestone 6 in `tasks.md`)
- Invokes the CLI
- Posts the Markdown report as a comment on the PR
- Updates the comment on subsequent runs rather than creating duplicates
- Sets the Action's conclusion based on the CLI exit code

The Action must not contain detection logic. It is a wrapper, not a second implementation.

### 8. Optional dbt impact module *(secondary — v1 if time permits)*

Parses a dbt `manifest.json` and builds a lookup of which models reference which tables and columns. When invoked, intersects that lookup with the set of tables/columns affected by the migration and produces findings for likely downstream breakage. Must be cleanly excludable from the build if it is deferred to v1.5.

**Packaging decision is deferred to the start of Milestone 9.** The choice between a runtime-flagged module (compiled in, inactive unless `--dbt-manifest` is passed) and a build-time-excluded module (linked only under a dbt build tag) is made at the start of M9 in `tasks.md`. Milestones 1–8 treat the dbt module as entirely absent — no dbt code, dependency, manifest parsing, or analyzer hook exists in those milestones' deliverables — which keeps those milestones unaffected regardless of the packaging outcome.

---

## Component responsibilities

| Component | Owns | Does not own |
|---|---|---|
| CLI entry | Flag parsing, orchestration, exit codes, signal handling | SQL execution, plan diffing, snapshot restore |
| Shadow DB runner | Provisioning, restoring, tearing down the Docker-based ephemeral DB | Migration execution, analysis |
| Migration executor | Running migration statements, transaction wrapping, stop-on-first-error behavior, per-statement timing / errors | Interpreting SQL, producing findings |
| Lock analyzer | Sampling locks, producing lock findings | Migration execution, report format |
| Query plan analyzer | Plan-only `EXPLAIN` capture + diffing, producing plan findings (only when executor succeeds) | Migration execution, running queries via `EXPLAIN ANALYZE` |
| Report generator | Text, JSON, and Markdown report rendering | Analysis logic, output delivery |
| GitHub Action wrapper | CI integration, PR comment posting, input resolution | Any detection logic |
| dbt impact module | Manifest parsing and downstream intersection | Anything non-dbt |

---

## Inputs and outputs

### Inputs consumed

- **Migration SQL** — one file or a directory of files, applied in order. Wrapped in a single transaction by default; explicit in-file `BEGIN`/`COMMIT` is respected.
- **Snapshot source** — a path to a Postgres dump file (`.sql`, `.dump`, or `.tar`) that the runner restores into a freshly provisioned Docker-based shadow DB. v1 does **not** accept a connection string to an externally-managed Postgres instance; that mode is deferred to v1.5.
- **Config file** *(optional)* — lists top queries to run against baseline and post-migration state, and optional threshold overrides. YAML format. No DSL. **Threshold overrides scope ONLY the query-plan regression thresholds** (cost delta, estimated rows delta, scan-type rules) — lock severity thresholds and the full-table rewrite heuristic are committed constants in `internal/lockanalyzer` and are never user-configurable in v1. **When the config is absent**, plan analysis is silently disabled and the tool still runs lock analysis on the migration and exits on the normal M1–M3 rules; when present but malformed, the tool fails with exit code 3; when present with zero top queries, plan analysis is a no-op.
- **dbt `manifest.json`** *(optional, secondary capability)* — standard dbt artifact.
- **Environment / flags** — output format, verbosity, report destination, GitHub token (Action mode).

### Outputs emitted

- **stdout** — human-readable report with top-line verdict and grouped findings.
- **Optional JSON file** — structured report for machine consumption.
- **Optional Markdown file** — report sized and formatted for a GitHub PR comment.
- **Exit code** — reflects overall severity (see CLI behavior).
- **Cleanup guarantee** — ephemeral Postgres instance and temporary files removed on exit.

---

## CLI behavior

### Command shape

```
schemaguard check \
  --migration <path-to-sql-file-or-dir> \
  --snapshot  <path-to-dump-file> \
  [--config   <path-to-config>] \
  [--dbt-manifest <path>] \
  [--format   text|json|markdown] \
  [--out      <path>] \
  [--verbose]
```

### Required inputs

- `--migration` — path to the migration SQL file or directory
- `--snapshot` — path to a Postgres dump file the runner restores into an ephemeral Docker-provisioned shadow DB

### Optional inputs

- `--config` — config file with top queries and threshold overrides
- `--dbt-manifest` — dbt manifest path (only used if secondary capability is enabled)
- `--format` — output format (default: `text`)
- `--out` — write the report to a file instead of stdout
- `--verbose` — extra logging for debugging runs

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Green — no significant findings |
| `1` | Yellow — caution-level findings, merge with care |
| `2` | Red — stop-level findings, do not merge (includes a migration that halted on its own SQL error) |
| `3` | Tool error (bad inputs, snapshot restore failure, Docker unavailable, crash) |

Exit code `3` must be clearly distinguishable from a red verdict so CI can tell the difference between "the tool says stop" and "the tool broke." A migration that halts on its own SQL error exits with `2`, not `3` — that is a product finding, not a tool malfunction.

---

## GitHub Action behavior

The Action wraps the CLI and does the following:

1. **Trigger** on pull requests that touch migration paths (configurable via Action input).
2. **Resolve inputs** — migration path, snapshot location, optional config path. The exact set of supported snapshot sources for v1 is closed during Milestone 6 in `tasks.md` and must be in place before substantive Action work begins.
3. **Provision** — run the CLI, which stands up a Docker-based ephemeral Postgres inside the Action runner.
4. **Capture output** — produce the Markdown report file via `--format markdown --out`.
5. **Post PR comment** — upsert a single comment per PR (update in place on re-runs) using the GitHub API and the provided token.
6. **Set Action conclusion** — success on exit 0, neutral/warning on exit 1, failure on exit 2, failure on exit 3.

The Action should fail closed: if it cannot post a comment, it should still surface the findings in the Action log so they are not silently lost.

The Action must **not** re-implement any detection logic. It is a shell around the CLI, and the CLI must remain the authoritative binary.

---

## Report structure

Every report — text, JSON, or Markdown — contains the same sections in the same order.

1. **Top-line verdict** — 🟢 Safe / 🟡 Caution / 🔴 Stop
2. **One-sentence summary** — plain-English description of the highest-severity finding.
3. **Findings grouped by category:**
   - **Migration Execution** *(rendered only when the migration halted on an error; carries the red migration-failure finding)*
   - **Lock Risk**
   - **Query Plan Regressions** *(empty or omitted when the migration halted on an error)*
   - **Downstream Impact** *(only if the dbt capability is enabled)*
4. **Per-finding fields:**
   - Affected object (table, index, column, query, model)
   - Severity (info / caution / stop)
   - Measured impact (lock duration, cost delta, scan type change, affected dbt model)
   - One-sentence "why this matters" explanation
5. **Footer** — tool version, run duration, shadow DB size, link to docs.

The Markdown variant must fit comfortably in a GitHub PR comment and must degrade gracefully if a category has zero findings (the section should be omitted, not rendered empty).

---

## Build order / milestone plan

Build strictly in this order. Do not start a milestone until the previous one is demonstrably working on the demo repository.

### Milestone 1 — Core pipeline (Week 1)

- CLI entry with flag parsing
- Shadow DB runner (Docker-based ephemeral Postgres, dump restore, teardown)
- Migration executor (single-transaction wrap with explicit `BEGIN`/`COMMIT` respect, statement-by-statement apply, per-statement timing, stop-on-first-error behavior)
- Lock analyzer (parallel sampling, basic findings)
- Minimal text report on stdout
- Exit codes wired to overall severity

**Done when:** `schemaguard check` runs on a trivial migration against a small dump and reports lock findings correctly. A deliberately failing migration stops cleanly on the first error, preserves any lock findings produced beforehand, and returns a red failure finding without a crash.

### Milestone 2 — Query plan regression + structured output (Week 2)

- Query plan analyzer (plan-only `EXPLAIN` baseline + post capture, diffing logic)
- Skip post-capture when the migration executor halted on error
- Config file loader (top queries, thresholds)
- JSON report output
- Improved text report formatting with grouped findings

**Done when:** The tool catches a renamed-column plan regression and a non-concurrent index lock on the demo repository, end-to-end, in one command. A deliberately failing migration still produces a correct lock-findings report with no plan-regression section and a red migration-failure finding.

### Milestone 3 — GitHub Action + Markdown report (Week 3)

- Markdown report variant
- GitHub Action wrapper
- PR comment upsert logic
- Action marketplace metadata (name, description, icon)
- End-to-end test on a real PR in the demo repository

**Done when:** Opening a PR in the demo repository triggers the Action and posts a correct, readable comment within a CI run.

### Milestone 4 — Demo repo hardening + release polish (Week 4)

- Public demo repository with four canonical failure-mode migrations (five if dbt ships)
- README with quickstart, install command, animated GIF of the PR comment, Squawk/Atlas acknowledgement
- Installation path (single-command install)
- Version tag, license, contributing notes
- Signal handling, cleanup guarantees, graceful error messages

**Done when:** A first-time user can clone the demo repo, run one command, and see four red findings within 60 seconds without reading docs.

### Secondary — dbt impact module *(ship only if M1–M4 are stable and time remains)*

- dbt manifest parser
- Affected-model intersection
- Fifth demo migration (dropped column referenced by a dbt model)
- Markdown report "Downstream Impact" section wiring

If this milestone is not complete by the launch date, drop it cleanly and move to v1.5 — see below.

---

## v1.5 / later items

The following are explicitly deferred. They must not appear in v1 unless a core milestone is ahead of schedule.

- **dbt impact module** (if not shipped in v1)
- **External Postgres mode for the shadow DB runner** — pointing the tool at an already-running Postgres instance instead of provisioning via Docker. v1 is Docker-only.
- Cloud-storage snapshot sources for the GitHub Action (S3, GCS, Azure Blob, etc.) beyond whatever limited set is chosen in Milestone 6.
- Automatic top-query discovery from `pg_stat_statements`
- Configurable threshold DSL beyond simple overrides
- Support for non-Postgres databases
- Historical tracking of risk across past migrations
- ML-based or learned risk scoring
- Rollback planning or auto-remediation
- Slack, Jira, or Teams integrations
- A web dashboard or hosted SaaS
- Multi-environment or multi-snapshot comparison
- Data masking or anonymization helpers

---

## Technical constraints

1. **Single authoritative binary.** All detection logic lives in one place. The GitHub Action is a wrapper, not a second implementation.
2. **Small, modular codebase.** Each component in the "Primary components" list should be a single package or module with a narrow public surface.
3. **Stateless.** No database of past runs, no cache, no learned state. v1 is pure input-in, report-out.
4. **No ORMs, no web frameworks.** Direct Postgres driver calls. No HTTP server in v1.
5. **Minimal dependencies.** Prefer standard library where reasonable. Every added dependency is a liability.
6. **Deterministic output.** Given the same inputs, the tool must produce equivalent reports. Non-determinism in findings is a bug.
7. **Graceful failure.** Malformed SQL, missing files, unreadable dumps, unavailable Docker, and interrupted runs must produce clear errors and leave no orphaned containers or processes behind. A migration that halts on its own SQL error is a product finding (exit code 2), not a tool malfunction (exit code 3).
8. **Speed targets** as defined in `requirements.md`, Constraint 8. Do not prematurely optimize — but do not build anything that makes those targets unreachable on the demo path.
9. **No hidden network calls.** The tool must not phone home, emit telemetry, or fetch remote resources without explicit user consent in v1.
10. **No premature generalization.** Do not build abstractions for future databases, future output formats, or future analyzers. Build exactly what v1 needs and nothing more.

---

## Quality bar for v1

Before launch, the following must be true on a clean machine:

- The CLI installs with a single command.
- `schemaguard check` runs end-to-end against the demo repository without errors.
- All four core canonical failure modes in the demo repository produce correct findings and correct severity.
- A deliberately failing migration (in a dedicated test case) stops on the first error, returns a red migration-failure finding, preserves any lock findings produced before the failure, and skips post-migration plan capture entirely.
- The GitHub Action runs on a real pull request in the demo repository and posts a correct Markdown comment.
- Running the tool twice in a row on the same inputs yields equivalent reports.
- Forcing a crash (Ctrl-C) mid-run leaves no orphaned Postgres containers or temp files.
- The text report is scannable in under 30 seconds by an engineer who has never seen the tool before.
- The tool does not require any configuration to produce its first useful report.

If any of the above fails, v1 is not ready to launch regardless of calendar pressure.

---

## Open implementation questions

These are real, unresolved build questions. Each is owned by a specific milestone and must be answered in writing by the time that milestone closes. Questions closed in earlier milestones have been moved to `docs/DECISIONS.md` and are not repeated here.

1. **Query plan diff thresholds.** What absolute and relative cost-increase thresholds should trigger a yellow finding? A red finding? Initial defaults should be chosen from first principles, then tuned against real migrations during design partner pilots. Owned by Milestone 4 (task 4.6).
2. **Snapshot restore speed.** For larger dumps, restore time may dominate total runtime. Is streaming restore acceptable, or should v1 require a pre-restored shadow DB for large-snapshot customers? This is a performance question, not a mode question.
3. **Markdown comment size.** GitHub PR comments have size limits. How does the report degrade when there are dozens of findings? Truncation strategy must be defined before the Action milestone. Owned by Milestone 5 (task 5.6).
4. **Snapshot source for the GitHub Action.** The CLI is indifferent to where the snapshot comes from — it takes a file path. The Action must acquire that file inside the CI runner, which introduces the question of which sources to support (repo file path, HTTPS URL, Actions artifact, Docker image). Owned by Milestone 6 (task 6.1) and must be closed before substantive Action development begins.

These questions do not need perfect answers before starting the project — but each one must be answered in writing by the end of the milestone in which it first becomes load-bearing.
