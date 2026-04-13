# Tasks — SchemaGuard v1

## Purpose of this file

This file is the **execution plan** for building SchemaGuard v1. It translates `requirements.md` (product truth) and `build_spec.md` (implementation truth) into an ordered, checkable task list that a founder, engineer, or AI coding agent can work through sequentially.

It is not a planning document. It does not debate scope or design. Where this file and `requirements.md` / `build_spec.md` disagree, those two documents win.

Treat every task as concrete and actionable. If a task can't be started because a decision is missing, it is itself a decision task and should be logged as such.

---

## Execution principles

1. **Build strictly in milestone order.** Do not start a later milestone until the previous one is demonstrably working.
2. **No feature creep.** If a task idea falls outside `requirements.md`, it goes in `v1.5.md`, not here.
3. **Keep it shippable.** Every milestone should leave the system in a runnable, testable state.
4. **Stateless by default.** No persistent storage, no background processes, no hidden state.
5. **Cleanup is a feature.** Any task that provisions resources must ship with its teardown in the same task.
6. **Demo repo is the spec.** When in doubt about what "working" means, the demo repository is the reference.
7. **Decisions are tasks.** If a question blocks a task, write it as a decision task and resolve it in writing before moving on.

---

## Milestone overview

| # | Milestone | Core or Optional | Depends on |
|---|---|---|---|
| 1 | Foundation / project setup | Core | — |
| 2 | Shadow DB + migration execution | Core | M1 |
| 3 | Lock-risk detection | Core | M2 |
| 4 | Query-plan regression detection | Core | M2 |
| 5 | Report generation | Core | M3, M4 |
| 6 | GitHub Action integration | Core | M5 |
| 7 | Demo repo and end-to-end validation | Core | M5 (partial), M6 (full) |
| 8 | Hardening / launch readiness | Core | M7 |
| 9 | dbt downstream impact *(secondary)* | Optional — v1 if time remains, else v1.5 | M5 |

Milestones 3 and 4 can be worked on in parallel once Milestone 2 is complete.
Milestone 7 can begin scaffolding (schema + seed data) as early as Milestone 2.

---

## Milestone 1 — Foundation / project setup

**Goal:** A clean, compilable, testable project skeleton with a working `schemaguard --help`, so every later task has a stable home to land in.

### Tasks

- [ ] **1.1 Language / runtime decision** *(decision task)*
  - [ ] Evaluate Go, Rust, and Python against: single-binary distribution, Postgres driver maturity, Docker interop, speed to prototype.
  - [ ] Record the decision and rationale in a short `DECISIONS.md` entry.
  - [ ] Done: the choice is written down and every subsequent task assumes it.

- [ ] **1.2 Repository and project skeleton**
  - [ ] Initialize the repo with license (permissive OSS — MIT or Apache-2.0), README stub, `.gitignore`, `DECISIONS.md`.
  - [ ] Create the top-level module layout matching the primary components in `build_spec.md`.
  - [ ] Wire up build, format, lint, and test commands.
  - [ ] Done: a fresh clone + single build command produces a runnable binary.

- [ ] **1.3 CLI entry point skeleton**
  - [ ] Implement `schemaguard` root command with a `check` subcommand.
  - [ ] Implement `--help` and `--version`.
  - [ ] Define all planned flags from `build_spec.md` (`--migration`, `--snapshot`, `--config`, `--dbt-manifest`, `--format`, `--out`, `--verbose`) with clear descriptions but no behavior yet.
  - [ ] Done: `schemaguard check --help` prints a complete, correct flag list.

- [ ] **1.4 Exit code convention**
  - [ ] Implement the exit-code enum (0 green, 1 yellow, 2 red, 3 tool error).
  - [ ] Ensure every tool-error path returns exit code 3, never 2.
  - [ ] Ensure a product-level red finding (including a migration that halts on its own SQL error) returns exit code 2, never 3.
  - [ ] Done: a failing input (e.g. missing file) exits with 3 and a clear message.

- [ ] **1.5 Signal handling scaffold**
  - [ ] Install a signal handler for SIGINT / SIGTERM that triggers a cleanup hook registry.
  - [ ] No real cleanup yet — just the hook registry.
  - [ ] Done: pressing Ctrl-C during a no-op run exits cleanly with a non-zero code.

### Dependencies
None.

### Done when
A clean clone builds with one command, `schemaguard check --help` works, exit codes are wired, and `DECISIONS.md` records the language choice.

---

## Milestone 2 — Shadow DB + migration execution

**Goal:** Given a dump and a migration file, the tool can stand up a Docker-based ephemeral Postgres, restore the dump, apply the migration inside a single transaction (or respect explicit in-file `BEGIN`/`COMMIT`), stop cleanly on the first error, and tear everything down.

### Tasks

- [ ] **2.1 Shadow DB runner — Docker mode**
  - [ ] Start an ephemeral Postgres container with a unique name.
  - [ ] Wait for readiness.
  - [ ] Expose a connection handle to the rest of the pipeline.
  - [ ] Register a teardown hook that stops and removes the container even on panic.
  - [ ] Done: calling the runner produces a live, connectable Postgres and always removes the container on exit. External-Postgres mode is explicitly out of scope in v1 — see the v1.5 tasks list.

- [ ] **2.2 Snapshot restore**
  - [ ] Support `.sql`, `.dump`, and `.tar` dump formats via `psql` / `pg_restore` invocations.
  - [ ] Capture restore duration and surface errors clearly.
  - [ ] Done: a sample dump restores successfully into the shadow DB and schema is verifiable.

- [ ] **2.3 Migration executor**
  - [ ] Parse the migration SQL file (or directory) into individually executable statements.
  - [ ] **Wrap the full migration in a single transaction by default.** If the migration file contains explicit `BEGIN` / `COMMIT` statements, respect those instead of wrapping.
  - [ ] Apply statements in order against the shadow DB.
  - [ ] Capture per-statement wall-clock time.
  - [ ] **On the first SQL error, stop applying further statements, mark the run as failed, and return a structured failure result.** Do not continue past an error. Do not attempt to analyze a partially-applied schema.
  - [ ] Lock findings already produced for statements that did execute must be preserved and surfaced in the final report, alongside a red migration-failure finding.
  - [ ] The executor must signal its failure status to downstream stages so that post-migration plan capture (task 4.4) is skipped cleanly on a failed run.
  - [ ] Done: a trivial migration applies under a single-transaction wrap and per-statement timings are returned. A deliberately failing migration halts on the first error, produces a red migration-failure result, preserves lock findings from statements that ran before the failure, and signals downstream stages to skip plan capture.

- [ ] **2.4 Cleanup guarantees**
  - [ ] Integrate the shadow DB teardown with the signal handler from 1.5.
  - [ ] Verify that Ctrl-C mid-run removes the container and any temporary files.
  - [ ] Done: a forced kill during restore or migration leaves no orphaned containers and no temp files.

- [ ] **2.5 Docker availability detection** *(decision task)*
  - [ ] Detect Docker presence at startup. Because v1 is Docker-only, there is no external-Postgres fallback — the tool must fail fast with a clear, actionable error when Docker is unavailable.
  - [ ] Define the exact error message shown when Docker is unavailable.
  - [ ] Done: the message is approved and committed.

### Dependencies
Milestone 1.

### Done when
`schemaguard check --migration <file> --snapshot <dump>` restores the dump, runs the migration inside a single transaction (or respects explicit in-file transactions), prints timings, stops cleanly on the first error when one occurs, preserves any partial lock findings, and leaves no residue. Nothing is analyzed yet — this milestone is pure plumbing.

---

## Milestone 3 — Lock-risk detection

**Goal:** While the migration executor runs, a parallel analyzer samples lock state and produces structured lock findings.

### Tasks

- [ ] **3.1 Lock sampler**
  - [ ] Implement a sampler that queries `pg_locks` and `pg_stat_activity` at a configurable interval (default TBD in 3.5).
  - [ ] Run the sampler concurrently with the migration executor.
  - [ ] Stop the sampler cleanly when the migration finishes — including when the migration halts on an error.
  - [ ] Done: sampler produces a raw event stream during a test migration and halts cleanly on both successful and failed runs.

- [ ] **3.2 Lock attribution**
  - [ ] Attribute observed locks to the currently-executing migration statement using timestamps.
  - [ ] Because the migration runs inside a single transaction by default, all locks fall inside one transaction scope — attribute locks to the originating statement via start/end timestamp bracketing.
  - [ ] Done: a test migration with a known locking statement is correctly attributed in the output.

- [ ] **3.3 Lock finding producer**
  - [ ] Convert attributed lock events into findings with: affected object, lock type, duration, blocks-reads flag, blocks-writes flag, severity.
  - [ ] Classify severity based on duration thresholds and lock mode.
  - [ ] Done: every lock observed during a test migration produces a well-formed finding.

- [ ] **3.4 Full-table rewrite recognition**
  - [ ] Detect full-table rewrites based on *observed* behavior (execution time, rows touched, lock duration), not static rules.
  - [ ] Produce a specific finding type for table rewrites.
  - [ ] Done: `ADD COLUMN NOT NULL DEFAULT` on a seeded table produces a rewrite finding with measured duration.

- [ ] **3.5 Lock sampling frequency — default** *(decision task)*
  - [ ] Empirically choose a sampling interval that balances accuracy and overhead.
  - [ ] Record the chosen value and the reasoning in `DECISIONS.md`.
  - [ ] Done: the default is committed and documented.

### Dependencies
Milestone 2.

### Done when
Running the tool on a migration with a known long lock produces a correctly categorized lock finding with measured duration and affected object, returned as a structured data object. When a migration halts on error, lock findings produced for statements that ran before the failure are still emitted.

---

## Milestone 4 — Query-plan regression detection

**Goal:** Given a list of top queries, the tool captures plan-only `EXPLAIN` plans before and after the migration and produces regression findings. When the migration executor halts on error, the analyzer produces no findings for that run.

### Tasks

- [ ] **4.1 Config file format**
  - [ ] Define a minimal config schema (YAML) containing a list of top queries and optional threshold overrides.
  - [ ] No DSL. No nesting beyond what is strictly needed.
  - [ ] **Threshold overrides in the config apply ONLY to query-plan regression thresholds** (cost delta, estimated rows delta, scan-type rules). Lock severity thresholds and the full-table rewrite heuristic are NOT user-configurable in v1 — they live as committed constants in `internal/lockanalyzer/finding.go` and `internal/lockanalyzer/analyze.go` per `docs/DECISIONS.md`. The config schema must not expose them.
  - [ ] Document the schema in `README.md` once M8 begins.
  - [ ] Done: the schema is committed and a sample config lives in the repo.

- [ ] **4.2 Config loader**
  - [ ] Load and validate the config file.
  - [ ] Fail fast with clear errors on malformed input.
  - [ ] **Config-absent behavior** (required, matches `requirements.md` Constraint 10 "Useful without configuration"):
    - When `--config` is **not provided**, plan analysis is silently disabled. The tool still runs migration execution and M3 lock analysis and exits normally on the existing M1–M3 rules.
    - When `--config` is **provided but malformed** (unreadable file, invalid YAML, schema violation), the tool fails fast with exit code 3 and a clear error message.
    - When `--config` is **provided with zero top queries**, plan analysis is a no-op — no plan findings are produced and the run exits normally.
  - [ ] Done: a malformed config produces an exit-code-3 run with a useful error message, a missing config file path is an exit-code-3 error, and omitting `--config` entirely produces a clean run with no plan findings.

- [ ] **4.3 Baseline plan capture**
  - [ ] For each top query, run `EXPLAIN (FORMAT JSON)` against the shadow DB before the migration runs.
  - [ ] **Plan-only `EXPLAIN` — never `EXPLAIN ANALYZE`.** The analyzer must not execute the top queries against the shadow DB.
  - [ ] Store the raw plan JSON in memory keyed by query identifier.
  - [ ] Done: baseline plans are captured and available to the analyzer.

- [ ] **4.4 Post-migration plan capture**
  - [ ] Re-run `EXPLAIN (FORMAT JSON)` on the same top queries after the migration executor finishes **successfully**. **Plan-only `EXPLAIN` — never `EXPLAIN ANALYZE`.**
  - [ ] Handle queries that now error (e.g. renamed/dropped columns) and record them as a specific failure mode.
  - [ ] **Skip this task entirely if the migration executor halted on error (see 2.3).** In that case the plan regression analyzer produces no findings for this run, and only lock findings plus the migration-failure finding appear in the report.
  - [ ] Done: post-migration plans are captured when the executor succeeds; the capture is cleanly skipped when the executor failed, with no plan-regression findings produced in that case.

- [ ] **4.5 Plan diff logic**
  - [ ] Implement a deterministic diff over the plan JSON that detects:
    - scan type downgrades (Index Scan → Seq Scan, Bitmap Heap Scan → Seq Scan, etc.)
    - estimated cost increases beyond threshold
    - **estimated rows delta beyond threshold** (derived from plan estimates; never from `EXPLAIN ANALYZE` measurements)
    - new full-table scans where none existed
    - queries that now error outright
  - [ ] Each detection produces a structured plan finding.
  - [ ] Done: a renamed-column regression and a dropped-index regression both surface correct findings on test data.

- [ ] **4.6 Initial threshold defaults** *(decision task)*
  - [ ] Pick first-pass cost and rows thresholds from Postgres first principles.
  - [ ] Commit them as defaults overridable in the config.
  - [ ] Record reasoning in `DECISIONS.md`.
  - [ ] Done: defaults are committed and the analyzer is usable with zero config.

### Dependencies
Milestone 2. Can be built in parallel with Milestone 3.

### Done when
Given a config with top queries and a migration that regresses them, the tool produces structured plan-regression findings with severity and affected-query identification. When the migration executor halts on error, this analyzer cleanly produces no findings and does not attempt to run `EXPLAIN` against a partially-applied schema.

---

## Milestone 5 — Report generation

**Goal:** Findings from Milestones 3 and 4 (and, where applicable, a migration-failure finding) are rendered into text, JSON, and Markdown outputs with a consistent structure.

### Tasks

- [ ] **5.1 Unified report data structure**
  - [ ] Define a single in-memory report type: overall verdict, summary, grouped findings (including a Migration Execution group rendered only when the migration halted on an error), footer metadata (version, runtime, shadow DB size).
  - [ ] All formatters consume this structure — none generate output directly from raw findings.
  - [ ] Done: the data structure is defined and populated at the end of a test run, including a run that halted on a migration error.

- [ ] **5.2 Verdict computation**
  - [ ] Compute the top-line verdict from the highest-severity finding: green / yellow / red.
  - [ ] Map the verdict to the correct exit code. A run that halted on a migration error maps to red (exit 2), not tool error (exit 3).
  - [ ] Done: verdict and exit code agree across all test scenarios, including the failed-migration scenario.

- [ ] **5.3 Text formatter**
  - [ ] Render a scannable stdout report: verdict emoji, one-sentence summary, grouped findings, footer.
  - [ ] Omit empty sections cleanly.
  - [ ] Done: a full test run prints a report a new user can understand without documentation.

- [ ] **5.4 JSON formatter**
  - [ ] Render the same report as a stable JSON document.
  - [ ] Schema must be versioned via a top-level `schemaVersion` field.
  - [ ] Done: the JSON validates against a simple schema check and can be consumed by an external script.

- [ ] **5.5 Markdown formatter**
  - [ ] Render a PR-comment-sized Markdown report with headings, severity icons, and a compact table per finding group.
  - [ ] Handle the case where a section is empty (omit it).
  - [ ] Truncate gracefully if the report is very large — details in 5.6.
  - [ ] Done: a sample Markdown file renders correctly when pasted into a GitHub PR comment.

- [ ] **5.6 Markdown truncation strategy** *(decision task)*
  - [ ] Define how the Markdown report degrades when findings exceed GitHub's PR comment size limit.
  - [ ] Record the strategy and limit in `DECISIONS.md`.
  - [ ] Done: large-finding-set test input produces a correctly truncated report with a clear "N additional findings omitted" footer.

- [ ] **5.7 Output writer**
  - [ ] Wire `--format` and `--out` flags to the correct formatter and destination (stdout or file).
  - [ ] Done: all three formats can be written to stdout or a file via the CLI.

### Dependencies
Milestones 3 and 4.

### Done when
A full run on a test migration produces all three output formats, with a correct verdict, correct grouping, correct severity, and the matching exit code — including the failed-migration scenario.

---

## Milestone 6 — GitHub Action integration

**Goal:** A thin Action wrapper runs the CLI in CI and posts the Markdown report as a PR comment.

### Tasks

- [ ] **6.1 Snapshot source resolution strategy** *(decision task — must be closed before 6.2 starts)*
  - [ ] Define the supported snapshot sources for the v1 Action. Candidate sources: (a) a repo-relative file path, (b) an HTTPS URL downloaded at runtime, (c) a GitHub Actions artifact from a prior job, (d) a Docker image layer containing the snapshot.
  - [ ] Document what happens when the snapshot is too large for a given source (e.g. repo file-size limits, HTTPS download timeouts).
  - [ ] Cloud-storage sources (S3, GCS, Azure Blob) are explicitly deferred to v1.5 unless a design partner requires otherwise.
  - [ ] Record the decision and rationale in `DECISIONS.md` before starting substantive Action development.
  - [ ] Done: the list of supported snapshot sources for v1 is written down and committed, and the rest of Milestone 6 proceeds against that list.

- [ ] **6.2 `action.yml` metadata**
  - [ ] Define name, description, branding, and input/output schema. Inputs must match the snapshot-source set chosen in 6.1.
  - [ ] Done: the Action is valid per GitHub's schema.

- [ ] **6.3 Action entrypoint script**
  - [ ] Check out the PR, resolve the migration path, resolve the snapshot location from the sources chosen in 6.1, invoke the CLI with `--format markdown --out <path>`.
  - [ ] No detection logic may live here — the CLI remains authoritative.
  - [ ] Done: the entrypoint reliably invokes the CLI and produces a Markdown file in the runner workspace.

- [ ] **6.4 PR comment upsert**
  - [ ] Post the Markdown report as a comment on the PR.
  - [ ] On subsequent runs, find the existing SchemaGuard comment and update it in place rather than creating duplicates. Use a hidden marker (HTML comment) for identification.
  - [ ] Done: pushing multiple commits to the same PR updates one comment, never creates a second.

- [ ] **6.5 Action conclusion mapping**
  - [ ] Map CLI exit codes to Action conclusions (success / neutral / failure).
  - [ ] Ensure exit code 3 (tool error) is distinguishable from exit code 2 (red verdict, including a failed migration).
  - [ ] Done: both a red verdict and a tool error produce the correct Action conclusion with a clear message.

- [ ] **6.6 Fail-closed logging**
  - [ ] If the comment cannot be posted (e.g. token missing), log the full Markdown report to the Action output so findings are not silently lost.
  - [ ] Done: revoking the token in a test run still surfaces findings in the Action log.

### Dependencies
Milestone 5. Task 6.1 must be closed before 6.2 starts.

### Done when
Opening a PR in a test repository triggers the Action, which runs the CLI end-to-end and posts or updates a correct, readable Markdown comment on the PR.

---

## Milestone 7 — Demo repo and end-to-end validation

**Goal:** A public demo repository exists with canonical failure migrations that exercise the tool end-to-end, both locally and in CI.

### Tasks

- [ ] **7.1 Demo schema and seed data**
  - [ ] Create an e-commerce schema: `users`, `orders`, `products`, `order_items`.
  - [ ] Seed with enough data to make lock and plan measurements meaningful (~1M rows in `orders`).
  - [ ] Provide a reproducible script that generates the dump from scratch.
  - [ ] Done: a fresh clone of the demo repo can produce the dump with one command.

- [ ] **7.2 Canonical failure migrations**
  - [ ] Add four migration PRs, each isolating one failure mode:
    1. `ADD COLUMN NOT NULL DEFAULT` (table rewrite)
    2. `CREATE INDEX` without `CONCURRENTLY` (write lock)
    3. `RENAME COLUMN` referenced by a top query (plan regression / broken query)
    4. `ADD CHECK CONSTRAINT` forcing a full table scan
  - [ ] Done: each PR, when run through the CLI locally, produces the expected finding type and severity.

- [ ] **7.3 Config file with top queries**
  - [ ] Ship a `schemaguard.yaml` config in the demo repo with the top queries referenced by the failure migrations.
  - [ ] Done: the config is committed and referenced by the demo README.

- [ ] **7.4 Local end-to-end run**
  - [ ] Verify one command clones the demo, runs the tool, and surfaces all four canonical findings in under 60 seconds on a modern laptop.
  - [ ] Done: a first-time user can reproduce this without reading documentation.

- [ ] **7.5 CI end-to-end run**
  - [ ] Wire the GitHub Action into the demo repo using the snapshot-source strategy chosen in 6.1.
  - [ ] Verify each failure-mode PR produces a correct Markdown comment.
  - [ ] Done: all four PRs have correct, readable, severity-accurate comments.

- [ ] **7.6 Demo repo README**
  - [ ] Write a short README for the demo repo: what it is, what the four migrations demonstrate, how to run the tool locally, link to the main repo.
  - [ ] Done: the README is committed and pointed to from the main repo.

### Dependencies
Milestones 5 (partial — for local runs) and 6 (full — for CI runs). Schema and seed scaffolding can start during Milestone 2.

### Done when
Both the local and CI paths catch all four canonical failure modes with correct severity, on a machine that has never seen SchemaGuard before.

---

## Milestone 8 — Hardening / launch readiness

**Goal:** The tool is ready to put in front of strangers. Rough edges are gone, install is clean, documentation is honest.

### Tasks

- [ ] **8.1 Single-command install**
  - [ ] Ship a single-command install path appropriate to the chosen language (e.g. `brew`, `go install`, `cargo install`, `curl | sh`).
  - [ ] Test it on a clean machine.
  - [ ] Done: a new user can install and run the tool in under a minute.

- [ ] **8.2 Main README**
  - [ ] Headline and tagline.
  - [ ] Animated GIF of the PR comment.
  - [ ] 30-second quickstart.
  - [ ] Philosophy paragraph (dynamic vs static).
  - [ ] Acknowledgement of Squawk and Atlas as complementary.
  - [ ] Supported today / explicitly not supported.
  - [ ] Install, usage, GitHub Action example.
  - [ ] Contributing, license, short roadmap.
  - [ ] Done: the README is committed and reviewed end-to-end by someone who has never seen the project.

- [ ] **8.3 Version tagging and license**
  - [ ] Tag v0.1.0.
  - [ ] Commit the OSS license chosen in 1.2.
  - [ ] Done: the repo has a proper first release.

- [ ] **8.4 Determinism test**
  - [ ] Run the tool twice on identical inputs and diff the JSON output.
  - [ ] Any non-determinism is a bug — fix before launch.
  - [ ] Done: two back-to-back runs produce equivalent reports.

- [ ] **8.5 Crash and cleanup test**
  - [ ] Force a crash (Ctrl-C, kill, out-of-memory) at three points: during restore, during migration, during analysis.
  - [ ] Verify no orphaned containers, processes, or temp files remain in any case.
  - [ ] Done: all three crash points clean up completely.

- [ ] **8.6 Clean-machine smoke test**
  - [ ] On a fresh VM or container, install the tool, clone the demo repo, run the tool, verify the report.
  - [ ] Done: the full path works with zero prior state.

- [ ] **8.7 Error message review**
  - [ ] Walk every error path (missing file, bad config, Docker unavailable, restore failure, migration syntax error, mid-migration SQL error) and verify each message is actionable.
  - [ ] Done: every error path has been exercised and its message approved.

- [ ] **8.8 Launch gate check**
  - [ ] Walk the "Quality bar for v1" checklist in `build_spec.md` end to end.
  - [ ] Any failing item blocks launch.
  - [ ] Done: every item passes.

### Dependencies
Milestone 7.

### Done when
The tool installs cleanly on a fresh machine, runs the demo repo end-to-end without errors, cleans up after crashes, produces deterministic output, and passes every item in `build_spec.md`'s "Quality bar for v1".

---

## Milestone 9 — dbt downstream impact *(secondary)*

**Goal:** *Ship only if Milestones 1–8 are complete and time remains before launch. Otherwise moves cleanly to v1.5.*

**Note on packaging.** The choice between a runtime-flagged dbt module (compiled in, inactive unless `--dbt-manifest` is passed) and a build-time-excluded dbt module (linked only when a dbt build tag is set) is made at the **start of this milestone**, not earlier. Milestones 1–8 treat the dbt module as entirely absent — no dbt code, dependencies, manifest parsing, or analyzer hooks exist in those milestones' deliverables. This keeps M1–M8 unaffected regardless of the final M9 outcome.

### Tasks

- [ ] **9.0 Packaging decision** *(decision task)*
  - [ ] Decide whether the dbt module is a runtime-flagged package or a build-time-excluded package.
  - [ ] Record the decision and rationale in `DECISIONS.md`.
  - [ ] Done: the decision is written down before any code from 9.1+ is started.

- [ ] **9.1 dbt manifest parser**
  - [ ] Load and parse a dbt `manifest.json`.
  - [ ] Build a lookup from (table, column) to referencing models.
  - [ ] Done: the parser handles a real-world manifest without error.

- [ ] **9.2 Affected-model intersection**
  - [ ] Given the set of tables and columns affected by the migration, identify referencing dbt models.
  - [ ] Produce structured downstream-impact findings with severity.
  - [ ] Done: a dropped column referenced by a dbt model produces a correctly scoped finding.

- [ ] **9.3 Downstream Impact report section**
  - [ ] Add the "Downstream Impact" section to all three formatters.
  - [ ] Section must be omitted entirely when the dbt capability is not enabled.
  - [ ] Done: findings render correctly in text, JSON, and Markdown.

- [ ] **9.4 Fifth canonical demo migration**
  - [ ] Add a fifth PR to the demo repo: dropping a column that a seeded dbt model depends on.
  - [ ] Include a minimal dbt project in the demo repo for this migration only.
  - [ ] Done: the fifth PR produces a correct downstream finding.

- [ ] **9.5 Launch or defer decision** *(decision task)*
  - [ ] If all of 9.1–9.4 pass the quality bar, ship in v1.
  - [ ] If not, cleanly exclude the dbt module from the v1 build path and move these tasks to `v1.5.md`.
  - [ ] Done: the decision is recorded in `DECISIONS.md`.

### Dependencies
Milestone 5.

### Done when
Either (a) the dbt module is fully integrated, tested end-to-end on the demo repo, and launch-ready, or (b) it is cleanly excluded and documented in `v1.5.md`.

---

## Optional / v1.5 tasks

These are explicitly **not** v1 work. Do not pull them forward.

- [ ] dbt downstream impact module (if not shipped in v1 via Milestone 9)
- [ ] **External Postgres mode for the shadow DB runner** — pointing the tool at an already-running Postgres instance instead of provisioning via Docker. v1 is Docker-only.
- [ ] Cloud-storage snapshot sources for the GitHub Action (S3, GCS, Azure Blob, etc.), beyond whatever limited set is chosen in task 6.1.
- [ ] Automatic top-query discovery from `pg_stat_statements`
- [ ] Richer threshold configuration (per-query overrides, severity tuning)
- [ ] Additional database engines (MySQL, SQL Server, etc.)
- [ ] Historical tracking of risk across past migrations
- [ ] Learned or ML-based risk scoring
- [ ] Rollback planning or auto-remediation
- [ ] Slack, Jira, or Teams notifications
- [ ] Web dashboard or hosted SaaS
- [ ] Multi-environment or multi-snapshot comparison
- [ ] Data masking or anonymization helpers

---

## Task dependency notes

- **M3 and M4 can run in parallel** after M2 completes. Both consume the shadow DB and migration executor but do not depend on each other.
- **M7 scaffolding can start early.** The demo schema, seed data, and canonical migrations can be drafted during M2–M5 so that M7 is primarily validation, not creation.
- **M6 cannot start** until M5's Markdown formatter is stable — the Action has nothing to post otherwise. Within M6, the snapshot-source decision task (6.1) must be closed before tasks 6.2–6.6 begin.
- **M8 cannot complete** until M7 has produced a reliable end-to-end run in both local and CI modes.
- **M9 is never a blocker.** Any slip in M9 pushes the dbt module to v1.5 rather than delaying launch. The M9 packaging decision (9.0) is made at the start of M9 and has no effect on M1–M8.
- **Decision tasks** (1.1, 2.5, 3.5, 4.6, 5.6, 6.1, 9.0, 9.5) must be answered in writing before the substantive tasks within their milestone begin.

---

## Definition of done for MVP

The MVP is considered shippable when **all** of the following are true:

1. Every task in Milestones 1 through 8 is complete and checked off.
2. The "Quality bar for v1" checklist in `build_spec.md` passes in full.
3. The demo repo produces all four canonical findings locally in under 60 seconds and in CI via the GitHub Action.
4. A first-time user can install the tool and run it successfully on the demo repo without reading more than the README.
5. Forcing a crash at any point leaves no orphaned containers, processes, or temp files.
6. Two identical runs produce equivalent reports.
7. A deliberately failing migration halts cleanly on the first error, returns a red migration-failure finding, preserves pre-failure lock findings, and skips post-migration plan capture.
8. Milestone 9 (dbt) is either fully complete and integrated, or cleanly excluded and documented in `v1.5.md`.
9. `DECISIONS.md` contains resolved entries for every decision task (1.1, 2.5, 3.5, 4.6, 5.6, 6.1, 9.0, 9.5).

If any item above is unfinished, the MVP is not done — regardless of calendar pressure.
