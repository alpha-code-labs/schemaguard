# Decisions — SchemaGuard

## Purpose of this file

This file is the short, stable record of decisions that are already made for SchemaGuard v1. It exists so that future implementation work — by a human or an AI coding agent — does not silently undo earlier strategic choices, and so that questions that have already been closed are not reopened.

It is not a requirements document, a build spec, or a planning doc. Those live in `requirements.md`, `build_spec.md`, and `tasks.md`. This file only records **what was decided**, **why**, and **what it implies** for implementation.

Update rules are at the bottom. Read them before editing.

---

## Confirmed decisions

### Decision: Postgres-only for v1
- **Status:** Confirmed
- **Decision:** The tool supports Postgres and only Postgres in v1. No MySQL, SQL Server, Snowflake, Databricks, MongoDB, or any other engine.
- **Why:** Every additional database engine multiplies implementation surface area for no wedge gain. Postgres is the target market for the first pilots and has the best `EXPLAIN` and lock-introspection ergonomics.
- **Implication:** Code, tests, demo repo, and documentation assume Postgres. No abstraction layer for "future databases" is permitted in v1.

### Decision: OSS-first distribution
- **Status:** Confirmed
- **Decision:** The core tool ships as open source under a permissive license (MIT or Apache-2.0). No paywalls or gated features in v1.
- **Why:** Distribution for an unknown founder in a dev-infra category depends on OSS trust and inbound. A closed-source or freemium v1 kills the cold-start path.
- **Implication:** No license checks, no telemetry, no phone-home, no feature flags that depend on a paid tier.

### Decision: CLI and GitHub Action are the only surfaces in v1
- **Status:** Confirmed
- **Decision:** The v1 product surface is a CLI binary and a GitHub Action wrapper. Nothing else.
- **Why:** A narrow surface is defensible to build, demo, and ship. Any additional surface (web UI, Slack bot, IDE plugin) is a distraction from the wedge.
- **Implication:** No HTTP server, no dashboard, no authentication, no hosted SaaS, no third-party integrations in v1. The Action is a thin wrapper and contains no detection logic.

### Decision: The wedge is dynamic verification, not static linting
- **Status:** Confirmed
- **Decision:** SchemaGuard's differentiator is that it runs the migration against a real shadow database and measures behavior. Static SQL linting is explicitly left to Squawk, Atlas, and similar tools and is not part of the v1 scope.
- **Why:** Static linting is a crowded and well-covered category. The underserved gap is dynamic verification on production-like data.
- **Implication:** Do not build a SQL linter. Positioning, docs, and feature work should respect the dynamic/complementary framing. Squawk and Atlas are acknowledged as complementary in the README.

### Decision: Core v1 detection pillars are lock risk and query plan regression
- **Status:** Confirmed
- **Decision:** v1 must ship two detection pillars: lock risk (real lock type and duration, full-table rewrites, blocking DDL) and query plan regression (plan diffs on user-specified top queries before and after the migration).
- **Why:** Both are universal pains for teams running Postgres at scale. Both can be built on top of the same shadow DB execution. They are the minimum surface area that makes the tool credible.
- **Implication:** Neither pillar can slip from v1. All other features are secondary.

### Decision: dbt downstream impact is secondary
- **Status:** Confirmed
- **Decision:** The dbt downstream impact capability ships in v1 **only if** Milestones 1–8 are stable and time remains. Otherwise it moves to v1.5. It must never block the release of the two core pillars.
- **Why:** dbt impact is contingent on the team using dbt, so it is less universal than the core pillars. It is also strategically important as a differentiator once the core is solid, but it is not the wedge.
- **Implication:** Milestones 1–8 treat dbt as entirely absent. No dbt code, dependencies, manifest parsing, or analyzer hooks exist in those milestones' deliverables. The go/no-go for shipping dbt in v1 is decided at the start of Milestone 9.

### Decision: Docker-only shadow DB mode for v1
- **Status:** Confirmed
- **Decision:** The shadow DB runner provisions a Docker-based ephemeral Postgres and nothing else in v1. Pointing the tool at an externally-managed Postgres instance is deferred to v1.5.
- **Why:** Docker-only halves Milestone 2's surface area. External mode adds no value on the demo path and only marginal value to first pilots. The earlier ambiguity between "must handle both modes" and "open question" was a real risk and is now closed.
- **Implication:** The CLI `--snapshot` flag takes a dump file path only; it does not accept a connection string. The tool must fail fast with a clear error when Docker is unavailable.

### Decision: Migration execution semantics — single transaction by default, respect explicit `BEGIN` / `COMMIT`
- **Status:** Confirmed
- **Decision:** The migration executor wraps the full migration file in a single transaction by default. If the migration file contains explicit `BEGIN` / `COMMIT` statements, those are respected instead of being wrapped.
- **Why:** This matches how Flyway and Alembic execute Postgres migrations in practice, which is how the user's production system will run them. It produces a coherent post-state to analyze and respects user intent when explicitly given.
- **Implication:** Code for the migration executor must implement the transaction wrap and the explicit-transaction detection path. No "per-statement autocommit" mode is supported in v1.

### Decision: Failure behavior — stop on first error, preserve partial lock findings, skip post-migration plan capture
- **Status:** Confirmed
- **Decision:** On the first SQL error during migration execution, the executor stops applying further statements, marks the run as failed, and returns a structured failure result. Lock findings produced before the failure are preserved and surfaced alongside a red migration-failure finding. Post-migration plan capture is skipped entirely on a failed run; the query plan regression analyzer produces no findings for that run.
- **Why:** "Continue where sensible" is not a specification. Once the tool wraps migrations in a transaction, this is the only coherent failure rule — any other rule analyses impossible hybrid states and produces misleading findings.
- **Implication:** The executor, lock analyzer, query plan analyzer, and report generator must all participate in the failure signal. A failed migration exits with code 2 (red), not code 3 (tool error), because it is a product finding.

### Decision: `EXPLAIN (FORMAT JSON)` plan-only, no `EXPLAIN ANALYZE` in v1
- **Status:** Confirmed
- **Decision:** The query plan regression analyzer uses plan-only `EXPLAIN (FORMAT JSON)`. `EXPLAIN ANALYZE` is not used in v1 and the analyzer never executes the top queries.
- **Why:** Plan-only is safer (no query side effects on the shadow DB), faster, and sufficient for the v1 wedge. `EXPLAIN ANALYZE` would require `SELECT`-only guardrails, concrete safety work, and meaningfully more engineering for marginal v1 value.
- **Implication:** The "rows" finding is labelled "estimated rows delta" (derived from plan estimates), not "rows examined." Any future move to `EXPLAIN ANALYZE` is a v1.5+ decision and must not be backdoored into v1.

### Decision: Users bring their own masked production snapshot
- **Status:** Confirmed
- **Decision:** The tool does not generate, mask, or anonymize production data. The user supplies a masked dump.
- **Why:** Masking is a deep, risky problem space and is not the wedge. Building a masking tool would consume months for marginal adoption lift.
- **Implication:** No masking code, no anonymization helpers, no PII detection in v1. Docs may point users to existing masking tools but must not ship one.

### Decision: Demo-path speed target — under 60 seconds on the demo and typical mid-size schemas
- **Status:** Confirmed
- **Decision:** A full run against the demo repository and typical mid-size schemas must complete in under 60 seconds on modern developer hardware. This is a target for the demo path and typical cases, not a universal promise. Larger real snapshots may take longer, but runtime should scale predictably with snapshot size.
- **Why:** The 60-second figure is load-bearing for the Show HN demo and the "one command, one minute, real findings" onboarding story. Treating it as a universal requirement would push the builder into accuracy-damaging shortcuts on large snapshots.
- **Implication:** Speed optimization work is prioritized for the demo path. Large-snapshot behavior is allowed to scale gracefully; the builder does not need to hit 60 seconds on multi-terabyte dumps.

### Decision: Config format is YAML
- **Status:** Confirmed
- **Decision:** The config file format (for top queries and optional threshold overrides) is YAML. No DSL, no deeply nested schemas.
- **Why:** YAML is familiar to the target audience, easy to parse, and does not require inventing a config language. A DSL is out of scope.
- **Implication:** A single YAML schema lives in the repo with a sample config. Schema evolution is deliberate and versioned.

### Decision: The GitHub Action contains no detection logic
- **Status:** Confirmed
- **Decision:** The Action is a thin wrapper around the CLI. All detection logic lives in the CLI binary. The Action's job is input resolution, CLI invocation, PR comment upsert, and conclusion mapping.
- **Why:** Two implementations of detection logic would drift. The CLI must remain authoritative.
- **Implication:** Pull requests that put detection logic in the Action are rejected. The Action is tested by running the CLI inside a runner, not by re-implementing checks.

### Decision: GitHub Action snapshot source resolution (v1 set)
- **Status:** Confirmed (resolved in Milestone 6, task 6.1)
- **Decision:** The v1 SchemaGuard GitHub Action supports exactly two snapshot sources, and the caller must set exactly one of them:
  - `snapshot-path` — a path to a Postgres dump file **relative to the PR checkout** (`$GITHUB_WORKSPACE`). Used for dumps that live in the repository.
  - `snapshot-url` — an **HTTPS URL** of a Postgres dump file. The Action downloads it via `curl --fail --location --max-time 600` into `$RUNNER_TEMP` before invoking the CLI. Used for dumps too large to commit to the repository. The 10-minute timeout is the only size guardrail the Action enforces; users managing multi-GB dumps should set their own workflow-level timeouts.
- **Why:** These two sources cover every real-world pattern we know about. `snapshot-path` handles the common case (demo repo, small committed dump, integration-test fixture). `snapshot-url` handles the "our dump is big and lives on a private HTTPS endpoint" case with no cloud-provider credentials or workflow-multi-job coordination. The two other candidate sources in `tasks.md` 6.1 — GitHub Actions artifacts from a prior job and Docker image layers carrying the snapshot — were considered and rejected for v1 because (a) Actions artifacts require a separate producer job, doubling workflow complexity, and (b) Docker-layer sources require registry authentication surface. Cloud-storage sources (S3, GCS, Azure Blob) are deferred to v1.5 per the existing `tasks.md` v1.5 list and will be added when a design partner's workflow requires them.
- **Implication:** The Action's input schema is exactly `migration`, `snapshot-path`, `snapshot-url`, `config`, `github-token`, and `go-version` — no cloud-credential inputs, no registry inputs, no artifact-name inputs. Any v1.5 extension that adds a new snapshot source must add a new input alongside these, not change their semantics. The mutual-exclusion check (exactly one of `snapshot-path` / `snapshot-url`) lives in `action/entrypoint.sh` and must run before any Docker or CLI work so users get a clear error before paying the cost of starting the shadow DB.

### Decision: Markdown report truncation strategy
- **Status:** Confirmed (resolved in Milestone 5, task 5.6)
- **Decision:** The Markdown formatter (`internal/report.FormatMarkdown`) renders the full report and, if the rendered string exceeds the committed budget of **55,000 characters** (`report.DefaultMarkdownBudget`), drops findings from the tail of the severity-sorted list until the output fits. The dropped findings are always the lowest-severity ones — stop-level findings are preserved until only the header/summary/footer remain. A notice of the form "⚠️ **Report truncated** — N additional lower-severity finding(s) omitted to fit the PR comment size budget." (`report.TruncationFooter`) is appended to the report whenever any finding was dropped. Pinned by `TestDefaultMarkdownBudgetMatchesDecision` and `TestFormatMarkdownTruncatesUnderSmallBudget`.
- **Why:** GitHub's PR comment size limit is 65,536 characters. 55,000 leaves a ~10,000-character headroom for (a) the M6 Action wrapper adding its own contextual header/footer before posting, (b) Markdown-to-rendered expansion, and (c) CJK or other multi-byte content in user-supplied query IDs or relation names. "Drop lowest-severity first" is the only rule that guarantees reviewers always see the loudest signal first — if we dropped from the front, a run with dozens of info findings could hide a stop finding behind truncation. The explicit truncation notice with a count is the minimum reviewers need to know they are looking at a partial report without having to parse the Markdown.
- **Implication:** Changes to the budget must update both `DefaultMarkdownBudget` and `TestDefaultMarkdownBudgetMatchesDecision`. The sorting order (`severity-desc → group-order → object-asc`) in `build.go` is load-bearing for truncation correctness — if that order changes, the "drop from the tail" behavior must be re-verified against the stop-level-preservation invariant. The truncation footer string is considered part of the public contract: the M6 GitHub Action and any external consumers grepping for truncation should rely on the "Report truncated" marker, so any rewording must treat it as a schema change.

### Decision: Verdict and exit-code aggregation live in the report layer
- **Status:** Confirmed (resolved in Milestone 5, tasks 5.1 and 5.2)
- **Decision:** `internal/report.Build` owns verdict computation: a failed migration is always `VerdictRed`, otherwise the verdict is the maximum severity across all findings (`stop → red`, `caution → yellow`, `info or none → green`). The CLI exits with `report.Verdict.ExitCode()`, which maps `green → 0`, `yellow → 1`, `red → 2`. This closes the M3/M4 deferral in which the CLI only emitted 0/2 based on `result.Failed` and ignored finding severity.
- **Why:** The verdict rule defined in `docs/tasks.md` 5.2 explicitly requires severity aggregation, and `docs/build_spec.md` Exit codes defines red as "stop-level findings, do not merge (includes a migration that halted on its own SQL error)." Both constraints are satisfied only once the report layer owns the computation — before M5 there was no single place that saw all three finding sources at once. Keeping it inside `internal/report` (not in `internal/cli`) means the same function produces the verdict for the text, JSON, and Markdown outputs, and future consumers like the M6 Action will read `report.Verdict` directly without reimplementing the rule.
- **Implication:** `internal/cli/check.go` must NEVER compute its own verdict — it must always return `rep.Verdict.ExitCode()`. Tool-level errors (bad input, Docker unavailable, restore failure, crash) still return `ExitToolError` (3) directly and never reach the report layer. Any future analyzer that adds a new finding severity beyond `info/caution/stop` must extend `report.Severity.Rank`, `verdictForSeverity`, and `Verdict.ExitCode` in lockstep. Pinned by `TestBuildFailedMigrationIsRedAndAddsMigrationExecutionFinding`, `TestBuildOnlyStopIsRed`, `TestBuildOnlyCautionIsYellow`, and `TestVerdictExitCodes`.

### Decision: Query plan diff thresholds (first-pass)
- **Status:** Confirmed (resolved in Milestone 4, task 4.6)
- **Decision:** `internal/planregression.DefaultThresholds` commits these first-pass values for the plan-regression analyzer:
  - **Cost ratio:** `caution ≥ 2.0x`, `stop ≥ 5.0x` — post-migration top-level `Total Cost` divided by baseline.
  - **Minimum cost delta:** `100` — absolute `(post - baseline)` floor. Both the ratio AND the delta must be exceeded before a cost-increase finding fires.
  - **Rows ratio:** `caution ≥ 10.0x`, `stop ≥ 100.0x` — post-migration top-level `Plan Rows` divided by baseline.
  - **Minimum rows delta:** `1000` — absolute floor for the rows finding, same shape as the cost floor.
  - **Scan-type regressions** are classified independently of the above thresholds: a new `Seq Scan` where the baseline had any index-style scan is always `stop`; any other scan-mode downgrade (`Index Scan → Bitmap Heap Scan`, etc.) is always `caution`; and a query that fails to `EXPLAIN` post-migration is always `stop` (`KindQueryBroken`).
- **Why:** Postgres planner costs are abstract units, so only ratios are meaningfully portable across workloads — a raw cost threshold would have to be workload-specific and would defeat "useful without configuration." 2x / 5x ratios give reviewers a loud signal for doubling and an actionable signal for a 5x jump. The absolute deltas (100 for cost, 1000 for rows) prevent false positives on tiny queries where the ratio is dominated by rounding noise. The rows thresholds are one order of magnitude looser than cost thresholds because estimated rows naturally fluctuate more than cost under statistic updates. Scan-type regressions do not use thresholds because a lost index is always worth escalating — the cost delta is already captured by the separate cost check.
- **Implication:** The six thresholds are the **only** plan-regression knobs exposed in the M4 YAML config. Every threshold can be overridden per-project via `thresholds.{caution,stop}_{cost,rows}_{ratio}` and `thresholds.min_{cost,rows}_delta`; any field omitted falls back to the default. Lock severity thresholds and the full-table rewrite heuristic are **not** exposed in the config (see the earlier M3 decision entries). The thresholds are committed Go constants rather than runtime defaults so there is exactly one place to change them; they will be tuned with real-migration pilot data. Pinned by `TestDefaultThresholdsMatchesDecision` in `internal/planregression/config_test.go` — any future change must update the test in lockstep.

### Decision: Lock sampling frequency default is 50 ms
- **Status:** Confirmed (resolved in Milestone 3, task 3.5)
- **Decision:** The lock sampler (`internal/lockanalyzer.Sampler`) polls `pg_locks` against the migration backend every **50 ms** by default, exported as `lockanalyzer.DefaultSamplingInterval`. The value is pinned by `TestDefaultSamplingIntervalMatchesDecision` so any future edit must update both the constant and this decision.
- **Why:** 50 ms captures locks as short as ~100 ms with at least one observation while keeping the sampler at ~20 queries/sec — a trivial load for a small `pg_locks` query against an ephemeral Postgres. Faster intervals (25 ms / 10 ms) meaningfully perturb their own measurements on busy migrations; slower intervals (100 ms / 250 ms) routinely miss brief `AccessExclusive` holds during fast metadata-only DDL. 50 ms is the balance point that reliably observes the canonical M3 demo case (`ADD COLUMN NOT NULL DEFAULT` holding `AccessExclusiveLock` for the rewrite) without adding meaningful overhead.
- **Implication:** The Sampler accepts a custom interval for tests and future tuning, but production runs always use the default. Any future change requires rerunning the M3 smoke test — a migration that triggers a full-table rewrite on a seeded table — and verifying the sampler still produces a `KindTableRewrite` finding. This default will be revisited with pilot data.

### Decision: Lock finding severity thresholds (first-pass)
- **Status:** Confirmed (first-pass defaults, resolved in Milestone 3)
- **Decision:** `internal/lockanalyzer.classifySeverity` uses these first-pass rules:
  - `AccessExclusiveLock`: `<100 ms` → info, `100–500 ms` → caution, `>500 ms` → stop
  - `ExclusiveLock`, `ShareRowExclusiveLock`: `<200 ms` → info, `200 ms–1 s` → caution, `>1 s` → stop
  - `ShareLock`, `ShareUpdateExclusiveLock`: `<1 s` → info, `1–5 s` → caution, `>5 s` → stop
  - `RowExclusiveLock`, `RowShareLock`, `AccessShareLock`: always info
- **Why:** These thresholds reflect the practical impact on concurrent application traffic in a typical OLTP Postgres deployment. `AccessExclusive` above 100 ms is already disruptive in production because it blocks every concurrent read; `Exclusive` is looser because it still allows `AccessShare` readers through; weaker locks are only a concern when held for seconds. The values are deliberately conservative (biased toward "caution" over "stop") so M3 can ship without over-classifying benign migrations.
- **Implication:** These are first-pass defaults, explicitly labeled as such. They will be tuned with real-migration data from design partners. The thresholds are first-class Go constants inside `classifySeverity` (not configurable in v1) so there is only one place to change them — the M4 config file intentionally does not expose them, per `requirements.md` "no configurable threshold DSL" non-goal.

### Decision: Full-table rewrite detection is observed-behavior, not SQL-pattern
- **Status:** Confirmed (resolved in Milestone 3, task 3.4)
- **Decision:** `internal/lockanalyzer.isLikelyTableRewrite` flags a probable full-table rewrite when **all** of the following hold: (1) the observed lock mode is `AccessExclusiveLock`; (2) the attributed statement took more than 100 ms of wall-clock time; (3) the observed lock duration covers at least 80 % of the statement window. The heuristic does not read the migration's SQL text.
- **Why:** `tasks.md` 3.4 requires that rewrites be detected from observed behavior, not static rules. A PG 11+ metadata-only `ALTER TABLE ADD COLUMN text` briefly holds `AccessExclusive` but finishes in milliseconds — reading the SQL alone would flag it as a rewrite incorrectly. A real rewrite (`ADD COLUMN NOT NULL DEFAULT`) holds the lock for the duration of the table rewrite, which scales with table size. The three-part heuristic separates the two without looking at SQL.
- **Implication:** Adding future lock-related findings (e.g. index-rebuild, concurrent index backfill) must extend `Analyze` with observed-behavior heuristics of the same shape, not SQL pattern matching. The 100 ms / 80 % thresholds are committed constants in `analyze.go` and will be tuned alongside the severity thresholds.

### Decision: Docker unavailability error message is committed
- **Status:** Confirmed (resolved in Milestone 2, task 2.5)
- **Decision:** When the Docker CLI is missing or the daemon cannot be reached, SchemaGuard fails fast with the exact message committed as `shadowdb.DockerUnavailableMessage` in `internal/shadowdb/availability.go`. The message tells the user that SchemaGuard v1 is Docker-only, that external Postgres support is deferred to v1.5, and lists the three concrete checks (Docker installed, daemon running, `docker version` succeeds). The CLI surfaces it verbatim via `shadowdb.CheckDockerAvailable` and exits with code 3 (tool error), never code 2 (product red).
- **Why:** v1 has no external-Postgres fallback, so Docker unavailability must be loud and actionable the first time a user hits it. Committing the exact wording in source (pinned by `TestDockerUnavailableMessageContainsActionablePhrases`) prevents silent drift and gives support a stable string to reference.
- **Implication:** Changes to the message must update both the constant and the test. Any future work that changes how Docker is detected must still surface this committed message on unavailability. The error wraps a sentinel `ErrDockerUnavailable` for callers that want to branch on it programmatically.

### Decision: dbt module packaging — deferred to v1.5
- **Status:** Confirmed (resolved at the end of Milestone 8, task 9.0)
- **Decision:** The dbt module packaging choice (runtime-flagged vs build-time-excluded) is **deferred to v1.5**. No dbt code, dependencies, manifest parser, or analyzer hooks exist in the v1 codebase.
- **Why:** Milestones 1–8 shipped cleanly and completely without dbt. Per `tasks.md` Milestone 9 task 9.5, this is the documented path: "If Milestones 1–8 are not stable or time is short, the dbt module moves cleanly to v1.5."
- **Implication:** v1 ships with no dbt surface. The JSON schema stays at `schemaVersion: "1"` with no `downstream_impact` group. The packaging decision is reopened when v1.5 begins.

### Decision: Ship-or-defer dbt for v1 — deferred to v1.5
- **Status:** Confirmed (resolved at the end of Milestone 8, task 9.5)
- **Decision:** dbt downstream impact detection is **deferred to v1.5**. It does not ship in v1.
- **Why:** Same rationale as task 9.0 above. The v1 detection pillars (lock risk + query-plan regression) are complete and verified. Adding dbt at this stage would delay launch with no evidence that v1 users need it before they can get value from the tool.
- **Implication:** See `v1.5.md` at the project root for the full deferred-items list.

### Decision: Language / runtime is Go
- **Status:** Confirmed (resolved in Milestone 1, task 1.1)
- **Decision:** SchemaGuard is written in Go. Module path: `github.com/schemaguard/schemaguard`. Go version: 1.22. Standard library only for Milestone 1 — no third-party dependencies unless a later milestone requires one.
- **Why:** Go produces a single static binary that is trivial to distribute via `go install`, Homebrew, and GitHub Releases — important for an OSS CLI targeting platform engineers. It has a mature, widely-used Postgres driver (`pgx`), a first-class Docker SDK for the shadow DB runner, and idiomatic CLI tooling patterns. It is faster to prototype than Rust and easier to ship as a single command than Python. Rust was a reasonable alternative but the distribution story is the same and the iteration speed is slower; Python loses on single-binary packaging.
- **Implication:** The project root is a Go module. Source lives under `cmd/schemaguard/` (binary entry) and `internal/` (packages matching the primary components in `build_spec.md`). Build command: `go build ./cmd/schemaguard`. Test command: `go test ./...`. Milestone 1 uses only the Go standard library; any third-party dependency introduced in a later milestone must be justified against Technical Constraint 5 in `build_spec.md` ("Every added dependency is a liability").

---

## Deferred / explicitly not in v1

The following are intentionally excluded from v1 and should not be pulled forward. Each exists because it has been considered and deferred, not because it was forgotten.

- Support for MySQL, SQL Server, Snowflake, Databricks, MongoDB, or any database other than Postgres
- External Postgres mode for the shadow DB runner (pointing at an already-running instance instead of Docker provisioning)
- `EXPLAIN ANALYZE` or any path that executes the top queries
- Data masking, anonymization, or PII detection
- Static SQL linting as a product feature (Squawk and Atlas cover this)
- Web dashboard, admin UI, hosted SaaS
- User accounts, authentication, teams, billing, pricing tiers
- Slack, Jira, Teams, or IDE integrations
- Rollback planning or automated remediation
- Historical tracking of risk across past migrations
- ML or learned risk scoring, heuristics that cannot be reproduced from raw plan JSON
- Automatic top-query discovery from `pg_stat_statements`
- Cloud-storage snapshot sources (S3, GCS, Azure Blob) beyond whatever limited set is chosen during Milestone 6
- Customer-specific risk rules or configuration DSLs
- Multi-environment (dev / staging / prod) or multi-snapshot comparison
- Full production workload replay

---

## Open decisions

The following decisions are **not yet made** and are owned by specific milestones. Do not treat them as resolved. Do not silently pick a default in code. When each decision is made, add it to the Confirmed Decisions section above and remove it from this list.

*All M1–M8 decisions are now resolved. The two M9 decisions below were closed as "deferred to v1.5" because Milestones 1–8 shipped cleanly without dbt, per the documented rule in `tasks.md` Milestone 9 ("If Milestones 1–8 are not stable or time is short, the dbt module moves cleanly to v1.5").*

No remaining open decisions.

---

## Change rule

This file is a stable reference. Keep it that way.

1. **Only update this file when a real decision is made.** A decision is "real" when it is reflected in at least one of `requirements.md`, `build_spec.md`, or `tasks.md`, and when it will materially affect implementation if undone.
2. **Do not silently assume a default for an open decision.** If an Open Decision is blocking work, resolve it in writing — in the owning milestone's decision task — and then move it into the Confirmed Decisions section here with a brief why.
3. **If `requirements.md`, `build_spec.md`, and `tasks.md` disagree, resolve the disagreement explicitly.** Update whichever of the three is wrong, and if the resolution changes a meaningful project decision, reflect it here. If it does not change a meaningful decision, do not add it to this file.
4. **Do not expand scope through this file.** New decisions that pull work into v1 from v1.5 (or vice versa) require a corresponding change in `requirements.md` or `tasks.md` first. This file records settled outcomes; it does not create them.
