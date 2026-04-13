# Things to Fix — Deferred Issues

## Purpose of this file

This file is the running record of **known issues and limitations that are intentionally not being fixed right now**. It exists so a future engineer (or an AI coding agent) can see at a glance:

- what is deferred and why
- when each item should be revisited
- what stage of the project the revisit belongs in

Items in this file have already been considered and consciously deferred. They are **not** general improvement ideas or product brainstorms — if an item on this list stops being useful, delete it rather than letting the file drift into a backlog.

New items should only be added when they meet one of these bars:
1. A real issue is found during implementation that is out of the current milestone's scope, or
2. A review turn (pre-flight audit, hygiene pass, post-milestone summary) flags it explicitly.

If a listed item is resolved, **delete its entry** — do not leave "(done)" stubs around. The "Recently resolved" section at the bottom exists only as a short-lived marker for items closed in the most recent hygiene pass; prune it when the next milestone starts.

---

## Deferred — per-query threshold overrides

**Issue.** The M4 YAML config has a single global `thresholds:` block that applies to every top query. Users cannot set a tighter cost ratio for one query and a looser one for another.

**Why not now.** The `requirements.md` non-goal "Customer-specific risk rules or configuration DSL" is explicit that per-query tuning is not v1 scope. Adding it would require a more nested schema, per-query merging logic, and additional validation surface — all disproportionate to the current signal we have about whether users actually need it. The six global thresholds committed in `docs/DECISIONS.md` are the first-pass defaults; design partners have not yet used the tool on real migrations, so we do not know what per-query pressure, if any, exists.

**When to revisit.** **Post-MVP / v1.5** — after at least three design-partner pilots have run the tool on real migrations. Only proceed if two or more of them independently ask for per-query tuning.

**Recommended stage.** v1.5.

---

## Deferred — automatic top-query discovery from pg_stat_statements

**Issue.** M4 requires the user to manually curate their top-query list in `schemaguard.yaml`. A future improvement would be to pull the top N queries automatically from `pg_stat_statements` (Postgres's built-in query statistics extension), eliminating the config step for the common case.

**Why not now.** Automatic discovery is already listed in the v1.5 section of `docs/tasks.md` and `docs/build_spec.md`. Committing to it now would require: (a) detecting whether `pg_stat_statements` is enabled in the user's database, (b) querying it safely, (c) deciding how "top N" is computed (by calls? by total time? by mean time?), (d) handling the case where the extension is not installed, (e) reconciling auto-discovered queries with the optional user-supplied list. None of this is needed until we have evidence that curating a handful of queries by hand is actually too much friction for users. The `requirements.md` Risk/Open-Question #2 is still unresolved and is explicitly the question this feature would answer.

**When to revisit.** **Post-MVP / v1.5** — after design partner pilots have produced evidence that manual curation is the main onboarding friction. If the first few pilots happily curate a 5–20 query list, this is not worth building.

**Recommended stage.** v1.5.

---

## Deferred — scan walker collapses to the "worst" scan per relation

**Issue.** `planregression.scansByRelation` walks the EXPLAIN JSON tree and records, for each relation, only the "worst" scan node it sees (ranked Seq Scan > Bitmap Heap Scan > Index Scan > Index Only Scan). If a plan references the same table in multiple subplans with different scan strategies (for example, a nested-loop rescan using the index while a sibling subquery does a seq scan), the walker collapses them and only surfaces the worst one.

**Why not now.** In practice, Postgres almost always uses a single scan strategy per relation within one query plan. The collapsing is usually conservative (the worst scan still surfaces in common single-scan plans, though multi-site regressions on the same relation can be hidden in more complex trees) and keeps the analyzer output small. The cost of fixing it is a noticeably more complex walker that emits per-scan-site findings, plus corresponding M5 report logic to group them readably. Building that now would be speculative.

**When to revisit.** **Only if real user pain appears** in the design partner pilots, OR if M5's structured report output makes it obvious that a single relation deserves multiple findings. If neither happens, leave it alone — collapsing is the right default for most plans.

**Recommended stage.** Only if real user pain appears, or as part of a later M5 output-refinement pass if the report output itself makes the limitation visible.

---

## Deferred — JSON schema and versioning policy

**Issue.** The M5 JSON report format exposes a single top-level `schemaVersion` field (currently `"1"`) and a hand-maintained layout in `internal/report/json.go`. There is no published schema document, no formal deprecation / compatibility policy, and no migration rules covering how and when `schemaVersion` bumps happen, what fields are safe to add at the same version, or how consumers should handle unknown fields. The one-line rule committed in `internal/report/build.go` — "any incompatible schema change must bump SchemaVersion" — is enough for the single internal consumer (the CLI itself) but is not enough for an external integration that wants to pin against a specific schema.

**Why not now.** The tool has no external JSON consumers yet. The only caller of `FormatJSON` is the CLI, and the only test pinning the schema is `TestFormatJSONHasSchemaVersionAndStableKeys`. Writing a formal versioning policy before a real consumer exists would be speculative — the policy would likely need to be rewritten once a real consumer's constraints are understood. v1 ships with the minimal contract (a `schemaVersion` field + a test pinning the current value); that is enough to prevent accidental silent drift without over-committing to a policy that nothing tests.

**When to revisit.** When the first external consumer depends on JSON stability. Concrete triggers: (a) the M6 GitHub Action or any CI-facing integration starts parsing the JSON rather than the Markdown; (b) a design partner writes a script around `schemaguard check --format json`; (c) a third-party tool (dashboards, compliance pipelines, observability) starts reading the output.

**Recommended stage.** Post-MVP / when the first external integration appears. Deliverables at that point: a published JSON schema document, an explicit compatibility policy (what "schemaVersion 1" guarantees, how additive changes work, what triggers a major bump), a deprecation window definition, and guidance for consumers on handling unknown fields. None of this should be written speculatively — wait until one concrete consumer's needs are known.
