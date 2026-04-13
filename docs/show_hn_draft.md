# Show HN Draft

Ready to post. Edit once more the morning of launch, then submit.

---

## Title

**Primary:**

```
Show HN: SchemaGuard – Runs your Postgres migration in CI and reports what will break
```

**Backup options** (in case the primary feels too long or a shorter
punch is needed):

```
Show HN: SchemaGuard – Catch unsafe Postgres migrations by actually running them
```

```
Show HN: SchemaGuard – Dynamic migration verification for Postgres, as a GitHub Action
```

The primary title follows the exact format from `docs/launch_plan.md`
and fits HN's ~80-character soft limit for readable titles. It
says what the tool does (runs your migration), where (in CI), and
what value it gives (reports what will break) — no buzzwords, no
hype, no "AI-powered."

---

## Submission URL

```
https://github.com/alpha-code-labs/schemaguard
```

Show HN posts link directly to the GitHub repo — not to a marketing
page, not to a blog post, not to a landing page. The README is the
landing page.

---

## First comment

Post this **immediately** after submitting. Do not wait. HN
rewards early engagement and a strong first comment sets the tone
for the entire thread.

---

Hey HN — I built this. I'm a solo technical founder.

**Why I built it:** I got tired of watching one-line migrations take
down production. A column addition that looks harmless in code
review silently locks a 50M-row table for 12 minutes. A renamed
column breaks a query that 10,000 customers hit every hour. These
problems can't be caught by reading the SQL — they only show up
when the migration hits real data at real scale. I wanted a tool
that actually runs the migration against a shadow copy of
production data and tells me what will happen, before I deploy.

**What it does:** SchemaGuard provisions a Docker-based ephemeral
Postgres, restores your masked production snapshot, executes the
migration inside a single transaction, samples `pg_locks` in
parallel to measure lock durations, and captures `EXPLAIN (FORMAT
JSON)` plans before and after to detect query-plan regressions. It
produces a red/yellow/green verdict and posts it as a PR comment
via a GitHub Action. The whole run takes about 20 seconds on a 1M-
row demo table.

**How it's different from existing tools:**

- **Migration runners** (Flyway, Alembic, Liquibase) execute
  migrations but don't verify them. SchemaGuard sits alongside them
  in CI and checks whether a migration is safe *before* the runner
  applies it.

- **Static linters** (Squawk, Atlas) read the SQL and flag known-bad
  patterns. They're great for catching syntax-level issues, but
  they're blind to anything that depends on actual data — they can't
  tell you that your table will lock for 12 minutes because they
  never touch the data. SchemaGuard actually runs the migration and
  measures what happens.

- **Datafold** does value-level data diffs in the analytics
  warehouse. Different layer entirely — SchemaGuard operates on the
  operational Postgres, not the warehouse.

SchemaGuard complements all three. Use Squawk for static linting,
SchemaGuard for dynamic verification, and your migration runner to
apply the changes you're confident about.

**What's in this release (v0.1.0):** Lock-risk detection (including
full-table-rewrite recognition), query-plan regression analysis,
text/JSON/Markdown reports, and a GitHub Action that upserts a PR
comment. Postgres only, Docker-only shadow DB, MIT licensed.

**What's NOT included yet:** dbt downstream impact detection,
automatic top-query discovery from `pg_stat_statements`, cloud-
storage snapshot sources, and multi-database support are all on the
roadmap but not shipped. I'd rather ship a sharp wedge than a broad
tool that half-works.

**What I'd love feedback on:**

1. Does this solve a real problem your team has?
2. What would make you actually try this on your own repo?
3. What failure modes am I missing?

The demo in the repo has four canonical bad migrations you can run
in one command to see what SchemaGuard catches. Clone it and try
`bash demo/run-demo.sh` if you want to see it in action before
reading further.

Thanks for looking.

---

## Why this wording matches the launch plan

| Launch-plan guidance | How this draft follows it |
|---|---|
| "who you are (1 sentence, no credentials padding)" | "I'm a solo technical founder." — no company name, no pedigree, no padding. |
| "why you built it" | The personal pain story: one-line migrations taking down production. Concrete, recognizable. |
| "what's different from Squawk/Atlas/Datafold in one honest sentence each" | Three bullet points, each naming the tool, saying what it does, and explaining the gap SchemaGuard fills. |
| "what feedback you want" | Three specific questions, not generic "feedback welcome." |
| Pain framing: "the specific, recognizable moment a platform engineer has lived through" | "A column addition that looks harmless in code review silently locks a 50M-row table for 12 minutes." |
| Avoid: "database observability / shift left / reliability platform / AI-powered / intelligent" | None of these appear anywhere in the draft. |
| Avoid: "Datafold for X" derivative framing | Datafold is mentioned as a "different layer entirely," not as a competitor being cloned. |
| Avoid: roadmap promises | "What's NOT included yet" is honest, concrete, and framed as "not shipped" rather than "coming soon." |
| "Thanks" | Last line. |

## Pre-post checklist

Before submitting, verify:

- [ ] Demo repo clones cleanly on a fresh machine
- [ ] `go install github.com/alpha-code-labs/schemaguard/cmd/schemaguard@v0.1.0` works
- [ ] `bash demo/run-demo.sh` produces all four findings
- [ ] The GitHub Action on PR #1 has a correct red comment
- [ ] The README renders the PR-comment screenshot correctly
- [ ] You are posting from your **personal** HN account (not a company alt)
- [ ] You have the full day clear for comment replies (7:30 AM – 11 PM PT)
- [ ] It is a **Tuesday or Wednesday**
