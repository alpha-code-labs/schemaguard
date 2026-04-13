# Requirements — Schema Migration Verification

## Product Name / Working Title

**SchemaGuard** *(working title — final name TBD)*

---

## Overview

SchemaGuard is an open-source CLI and GitHub Action that runs a proposed Postgres schema migration against a production-like shadow database **before** it is deployed and reports the real risks the migration introduces.

It is built for platform and database engineers who ship migrations to large Postgres databases and want to know — with confidence, not guesswork — whether a migration is safe to merge.

---

## Problem Statement

Every engineering team running Postgres at scale ships schema migrations under uncertainty. A migration that looks harmless in code can:

- lock a production table for minutes
- degrade the performance of business-critical queries
- silently break downstream analytics and dbt models

Existing tools either **run** migrations without verifying them (Flyway, Liquibase, Alembic) or analyze the SQL **statically** without touching real data (Squawk, Atlas). Neither answers the question engineers actually care about:

> "What will this migration do when it runs on our real production data?"

The result is a chronic, painful gap: teams ship migrations scared, react to incidents after the fact, and have no standard way to catch problems in advance.

---

## Target User

**Primary user (v1):**
Platform engineers, database reliability engineers (DBREs), and staff/principal engineers at companies running Postgres in production. They own migration safety, have usually been burned by a previous incident, and are technical buyers who can install and run an OSS CLI on their own authority.

**Eventual buyer (post-MVP):**
Engineering leaders — Head of Platform, Head of Infrastructure, VP Engineering — who already hold budget for reliability and will sponsor a paid tier once the tool has proven itself through usage.

---

## Core Value Proposition

**Run your Postgres migration against production-like data and find out what will break — before deployment.**

One command (or one CI run) executes the migration against a shadow database built from the team's own masked production snapshot, measures real behavior, and returns a clear verdict: **safe, caution, or stop** — with specific reasons.

---

## MVP Scope

The MVP is built around **two core detection pillars** and one **secondary capability**. The two core pillars must ship in v1. The secondary capability ships in v1 only if time permits; otherwise it moves to v1.5 and must not block release of the two core pillars.

### Required inputs

- A Postgres connection string or a path to a masked production dump the user provides
- A migration SQL file (or directory of migration files)
- An optional config file listing the team's top queries
- An optional path to a dbt `manifest.json` (used only if the secondary capability is enabled)

### Shadow database provisioning

- Restore the user-supplied snapshot into an ephemeral local Postgres instance
- Apply the migration against it

### Core detection pillar 1 — Lock risk *(must ship in v1)*

- Measure real lock type and duration during migration execution
- Flag long locks, full table rewrites, and blocking DDL patterns

### Core detection pillar 2 — Query plan regression *(must ship in v1)*

- Run `EXPLAIN` on each user-specified top query before and after the migration
- Flag regressions such as index scan → sequential scan, cost increase beyond threshold, new full-table scans, and queries that break outright

### Secondary capability — Downstream dbt impact *(ship in v1 if time permits, otherwise v1.5)*

- Parse a dbt `manifest.json` if provided
- Identify dbt models that reference tables or columns affected by the migration
- Flag likely breakage
- This capability is **optional in early v1** and must not delay the release of the two core pillars.

### Structured report

- Red / yellow / green top-line verdict
- Grouped findings with per-finding severity and a one-line "why this matters"
- CLI output (human-readable + optional JSON)
- GitHub PR comment via the Action

### CI integration

- A GitHub Action wraps the CLI, runs on migration pull requests, and posts the report as a PR comment automatically

---

## Out of Scope / Non-Goals

The MVP will **explicitly not** include any of the following:

- Support for databases other than Postgres (no MySQL, SQL Server, Snowflake, Databricks, MongoDB, etc.)
- A web dashboard, admin UI, or hosted SaaS
- User accounts, authentication, teams, billing, or pricing tiers
- Slack, Jira, Teams, or IDE integrations
- Data masking or anonymization of production snapshots — users bring their own
- Static SQL linting — Squawk and Atlas already cover this and are complementary
- Rollback planning or automated remediation
- Historical tracking, trend graphs, or ML-based risk scoring
- Full production workload replay — only the user-specified top queries
- Multi-environment support (dev / staging / prod) — one shadow database at a time
- Customer-specific risk rules or a configuration DSL

---

## Primary User Workflow

1. An engineer opens a pull request containing a Postgres migration file.
2. The GitHub Action triggers on the PR.
3. The Action provisions an ephemeral Postgres instance from the team's masked production snapshot.
4. The Action applies the migration to the shadow database while measuring lock behavior.
5. The Action runs the configured top queries before and after the migration and compares plans.
6. If a dbt `manifest.json` is configured and the secondary capability is enabled, the Action identifies affected downstream models.
7. The Action posts a Markdown report as a PR comment.
8. The engineer reads the report and decides whether to merge, revise, or reject the migration.

The same flow is available locally via the CLI, pointing at any dump file and migration file.

---

## Key Outputs

### CLI output

- Human-readable structured report on stdout
- Optional JSON output for machine consumption
- Exit code reflecting overall severity (0 = green, non-zero = yellow/red)

### PR comment output

- **Top-line verdict:** 🟢 Safe / 🟡 Caution / 🔴 Stop
- **One-sentence summary**
- **Grouped findings:**
  - Lock Risk
  - Query Plan Regressions
  - Downstream Impact *(only if the secondary dbt capability is enabled)*
- **Each finding includes:** affected object, severity, measured impact (e.g. lock duration, cost delta), one-line explanation of why it matters

The entire report must be readable in under 30 seconds.

---

## Success Criteria

### Product success criteria *(things the MVP itself must achieve)*

1. An engineer can install the CLI, run it against the demo repository, and receive a complete report in under 60 seconds on a modern laptop. On real production-sized snapshots, runtime should remain within the bounds of a normal CI job and a tolerable local dev wait.
2. The tool correctly identifies the core canonical failure modes in the demo repository:
   - Long-lock table rewrite from an `ADD COLUMN NOT NULL DEFAULT`
   - Non-concurrent index creation causing write locks
   - Renamed column breaking a top query / causing plan regression
   - Check constraint causing a full table scan on validation

   A fifth case — dropped column referenced by a dbt model — is included in the demo only if the secondary dbt capability ships in v1.
3. A platform engineer seeing the report for the first time can understand what is wrong and why, without reading documentation.
4. The GitHub Action runs end-to-end on a real pull request and posts a clean, correct comment.

### Early validation signals *(evidence the product matters — tracked separately, not gating)*

- At least one external user voluntarily runs the tool against a real production migration and reports back on what it caught or missed.
- At least one platform engineer describes, unprompted, a real incident the tool would have prevented.
- At least one design partner commits to running the tool on every migration PR for a defined pilot period.

Validation signals are tracked separately from product success criteria because they depend on external adoption, not on the correctness of the build.

---

## Constraints and Product Principles

1. **Postgres-only for v1.** No other database engines until Postgres is clearly winning.
2. **OSS-first.** The core tool is open source under a permissive license. No paywalls or gated features in v1.
3. **Dynamic verification only.** The wedge is running the migration against real data. Static analysis is left to existing tools.
4. **Bring-your-own snapshot.** The tool does not generate, mask, or synthesize production data.
5. **Complementary, not replacement.** SchemaGuard sits alongside Flyway, Liquibase, Alembic, Atlas, and Squawk — it does not replace migration runners or static linters.
6. **No platform ambitions in v1.** No dashboard, SaaS, accounts, or billing.
7. **Narrow surface area.** The CLI and the GitHub Action are the only user-facing surfaces.
8. **Fast feedback.** The tool must be fast enough to be useful both in CI on migration PRs and in the local developer inner loop. As a concrete target, a run against the demo repository and typical mid-size schemas should complete in under 60 seconds on modern developer hardware. Larger real snapshots may take longer, but runtime should scale predictably with snapshot size.
9. **Readable by default.** The report must be scannable in under 30 seconds by a tired engineer on a Friday afternoon.
10. **Useful without configuration.** The tool must be immediately useful on day one without any tuning. Initial thresholds are first-pass defaults based on Postgres internals and known-unsafe patterns; they are expected to evolve as real usage data comes in from design partners. Users should not be required to configure anything to see value on their first run.

---

## Differentiation

**Vs. migration runners (Flyway, Liquibase, Alembic).**
These tools *execute* migrations; they do not *verify* them. SchemaGuard is meant to run alongside them in CI, before deployment.

**Vs. static linters (Squawk, Atlas lint, Bytebase lint).**
Static linters read the SQL and flag known-unsafe patterns without ever touching data. They are fast and cheap but blind to anything that depends on real data volume, real workloads, or real downstream dependencies. SchemaGuard runs the migration against an actual shadow database and measures behavior.

**Vs. data diff tools (Datafold).**
Data diff tools compare values before and after a transformation, primarily in analytics warehouses. SchemaGuard focuses on the operational Postgres layer and on migration-specific failure modes — locks, query plans, and schema-level downstream impact — not value-level diffing.

**The narrow wedge:**
Dynamic verification of a Postgres migration against production-like data, with lock risk and query plan regression findings (and, where applicable, downstream impact) delivered as a PR comment in CI.

---

## Risks / Open Questions

1. **Snapshot prerequisite.** Users must provide a masked production snapshot. Teams that do not already have one face a non-trivial step before getting value. How much friction does this add in real adoption, and should we ship guidance or helpers (not full masking) to reduce it?
2. **Query workload specification.** The tool depends on users declaring top queries or extracting them from `pg_stat_statements`. Will teams actually curate this list, or will they expect automated discovery in v1?
3. **Budget conversion.** Migration fear is chronic, not acute. Teams may love the tool in demos but hesitate to pay for a recurring subscription. How do we validate willingness to pay during design partner pilots before investing in a commercial tier?
4. **Incumbent expansion risk.** Atlas, Bytebase, and Datafold could plausibly add dynamic verification within 12–18 months. What depth of capability is required to stay defensible during that window?
5. **False positives.** Aggressive detection that produces noise will erode trust quickly. What is the acceptable false-positive rate, and how will we measure it against real migrations during pilots?
6. **dbt dependency.** Downstream impact detection requires a dbt `manifest.json`. Teams that do not use dbt get less value from that feature. Is dbt-only downstream coverage acceptable when the secondary capability does ship, or does it need to generalize earlier?
7. **Naming.** "SchemaGuard" is a placeholder and may collide with existing tools or trademarks. A final name decision is still open.
