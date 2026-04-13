# Things to Fix Before Launch

Pre-launch items identified by the launch-readiness audit against
`docs/launch_plan.md`. Listed in strict priority order. Each item
states the issue, why it matters, whether it blocks launch, and the
smallest concrete next action.

Do not treat this file as a backlog. These are the only items
standing between the verified v0.1.0 build and a credible public
launch. Fix them in order, then delete this file.

---

## 1. ~~Fix module path / repo URL mismatch~~ ✅ FIXED

**Resolved and publicly verified.** Option B was applied: the module
path was updated from `github.com/schemaguard/schemaguard` to
`github.com/alpha-code-labs/schemaguard` across go.mod, all internal
imports (~10 Go files), the README (install, clone, and Action
references), the Makefile ldflags, DECISIONS.md, and the demo
workflow comment. Option A (creating a `schemaguard` GitHub org) was
not feasible because the `schemaguard` org already exists on GitHub
and belongs to a different owner.

The repo has been made public. Public verification performed:

- GitHub API confirms `private: false`, `visibility: public`.
- GitHub release `v0.1.0` is publicly accessible (HTTP 200).
- Unauthenticated `git ls-remote` resolves the `v0.1.0` tag.
- `proxy.golang.org` returns HTTP 200 for the module info.
- **`go install github.com/alpha-code-labs/schemaguard/cmd/schemaguard@v0.1.0`
  succeeds with default GOPROXY, no GOPRIVATE, no GONOSUMCHECK** —
  the exact command a real user would run. Binary installs and runs.
- The README's `git clone` URL and Action `uses:` reference are
  correct for the public repo.

**No remaining launch-day action for this item.**

---

## 2. ~~Create animated GIF / visual demo asset~~ ✅ FIXED

**Resolved.** A real screenshot of the actual SchemaGuard PR comment
from [PR #1](https://github.com/alpha-code-labs/schemaguard/pull/1)
was captured using headless Chrome against the GitHub-rendered HTML
of the comment body. The image is at `assets/pr-comment.png` (920×600
PNG, 72 KB) and is embedded in the README immediately after the
tagline blockquote — the launch-plan's "hook" position.

The image shows the real 🔴 Stop verdict, the Query Plan Regressions
table with both `orders_by_customer` and `recent_orders` broken-query
findings, the footer with version and timing, and the
`github-actions[bot]` attribution. Every pixel comes from the real
product running on a real PR — nothing is mocked.

**Why a static image instead of an animated GIF:** An animated GIF of
the full PR-opening flow requires an interactive browser screen
recording, which this CLI environment cannot produce. The static
screenshot captures the highest-value frame (the red PR comment with
findings) and functions as a strong visual hook in the README. If
desired, the founder can later replace it with a full animated
recording by opening a browser to PR #1, recording the scroll to the
comment, and saving as a GIF — but the current image is truthful,
real, and sufficient for launch.

---

## 3. ~~Take screenshots~~ ✅ FIXED

**Resolved.** Both launch-plan screenshots now exist in `assets/`:

1. **`assets/pr-comment.png`** (920×600, 72 KB) — hero screenshot of
   the real SchemaGuard PR comment from PR #1. Shows 🔴 Stop verdict,
   Query Plan Regressions table, both broken-query findings, footer,
   and bot attribution. Already embedded in the README as the visual
   hook (done in item #2). Doubles as the hero screenshot asset.

2. **`assets/cli-output.png`** (900×620, 107 KB) — terminal
   screenshot of a real `schemaguard check` run against demo migration
   01 (ADD COLUMN NOT NULL DEFAULT with volatile default). Shows the
   command, the 🔴 STOP verdict, the Lock Risk section with five
   findings at stop/caution severity, and the footer. Rendered from
   real CLI output via a styled terminal HTML page + headless Chrome.
   Available for Show HN posts, blog posts, and social sharing.

Both images are from real product runs — nothing is mocked.

---

## 4. Draft Show HN title and first comment

**Issue.** The Show HN post has not been drafted. The launch plan
says it should be "written and edited a week before launch." Hacker
News is the primary launch channel for a technical OSS tool
targeting platform engineers.

**Why it matters.** The title and first comment determine whether
the post gets traction. A well-crafted title stops the scroll; a
strong first comment earns upvotes and keeps the post on the front
page. You get one shot — a weak title or a rambling first comment
wastes the launch window.

**Blocks launch?** Does not block making the repo public. **Does
block the launch-day sequence** — you cannot post on HN without
writing the post.

**Smallest next action.** Draft exactly two things:

1. **Title:** `Show HN: SchemaGuard – Runs your Postgres migration
   in CI and reports what will break` (directly from the launch
   plan's suggested format).

2. **First comment:** One sentence on who you are (no credential
   padding). One sentence on why you built it. One honest sentence
   on how it differs from Squawk. One honest sentence on how it
   differs from Atlas. What feedback you want. Thanks. Keep it
   under 200 words. Edit it three times before launch day.

Takes ~30 minutes to draft, plus a few editing passes over the
following days.

---

## 5. Publish GitHub Action to Marketplace

**Issue.** `action.yml` exists at the repo root with correct
schema, branding (shield icon, blue), and inputs. But the Action
has not been published to the GitHub Marketplace. Without a
Marketplace listing, users can still use the Action via
`uses: <owner>/schemaguard@v0.1.0`, but they will not discover it
through GitHub's Marketplace search.

**Why it matters.** The Marketplace is a discovery channel. Platform
engineers searching for "postgres migration" or "schema check" in
the Marketplace would find SchemaGuard if it were listed. Without
the listing, the only discovery paths are the README, HN, and
direct links.

**Blocks launch?** No. The Action works without a Marketplace
listing. Users just reference it by repo path.

**Smallest next action.** Go to the repo's settings → Actions →
"Publish this Action to the GitHub Marketplace." Fill in the
description and categories. Takes ~5 minutes.
