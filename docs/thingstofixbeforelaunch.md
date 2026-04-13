# Things to Fix Before Launch

Pre-launch items identified by the launch-readiness audit against
`docs/launch_plan.md`. Listed in strict priority order. Each item
states the issue, why it matters, whether it blocks launch, and the
smallest concrete next action.

Do not treat this file as a backlog. These are the only items
standing between the verified v0.1.0 build and a credible public
launch. Fix them in order, then delete this file.

---

## 1. Fix module path / repo URL mismatch

**Issue.** `go.mod` declares `module github.com/schemaguard/schemaguard`.
Every internal import, every `go install` command in the README, the
`git clone` URL, and the GitHub Action `uses:` reference all use
`github.com/schemaguard/schemaguard`. But the actual repo lives at
`github.com/alpha-code-labs/schemaguard`. Go's module proxy resolves
the module path by fetching from the declared GitHub URL. Since
`github.com/schemaguard/schemaguard` does not exist as a repository,
every external `go install` and `go get` will fail with a 404.

**Why it matters.** This is the install path. If it does not work, no
one outside this machine can install or use the product.
`docs/launch_plan.md` mistake #1: "One broken install command on
Day 1 kills the launch."

**Blocks launch?** **Yes.** Hard blocker. No external user can install
until this is resolved.

**Smallest next action.** Either:

- **(a) Create a `schemaguard` GitHub organization** and transfer
  (or fork) the repo to `github.com/schemaguard/schemaguard`. This
  makes every existing reference (go.mod, README, Action) correct
  with zero code changes. GitHub automatically redirects the old URL
  after a transfer. Then retag v0.1.0 on the new repo.

- **(b) Update `go.mod` to `module github.com/alpha-code-labs/schemaguard`**,
  update every `import` statement across the codebase (~20 files),
  update the README install/clone/Action references, update
  `action.yml` and the demo workflow, re-commit, retag, and
  re-release. More invasive but does not require a new GitHub org.

Option (a) is cleaner. Option (b) is self-contained. Pick one and
execute before any public posting.

---

## 2. Create animated GIF / visual demo asset

**Issue.** The README has no animated GIF or demo video. The launch
plan (`docs/launch_plan.md` pre-launch checklist) says the GIF of
the PR comment appearing is "the hook" — the visual that stops the
scroll and shows the value proposition in 5 seconds. Currently the
README is text-only above the fold.

**Why it matters.** Platform engineers scroll quickly. A GIF showing
"open a PR → red comment appears → engineer reads findings" is the
single highest-leverage visual asset for the README and the Show HN
post. Without it, the first impression is a wall of text.

**Blocks launch?** No. The product works without it. But the README
loses its most impactful visual hook and the Show HN post has no
visual to share.

**Smallest next action.** Open PR #1 on the published repo (which
already has a real red SchemaGuard comment from the M8 verification
run). Screen-record the flow: navigate to the PR → scroll to the
comment → read the verdict and findings table. Save as a GIF (or
short MP4 converted to GIF). Embed in the README immediately after
the tagline blockquote. Takes ~20 minutes.

---

## 3. Take screenshots

**Issue.** The launch plan lists two screenshots:
- One hero screenshot of a PR comment with findings
- One terminal screenshot showing the CLI text output

Neither exists in the repo.

**Why it matters.** Screenshots provide quick visual evidence for
anyone reading the README, a Show HN post, or a tweet without
running the tool themselves. They are less impactful than the GIF
but still useful for static contexts (blog posts, newsletters).

**Blocks launch?** No. Secondary asset.

**Smallest next action.** Take a screenshot of the PR comment on
PR #1 (`alpha-code-labs/schemaguard#1`). Run `schemaguard check`
locally against demo migration 01 and screenshot the terminal
output. Save both as PNGs in an `assets/` or `docs/images/`
directory and optionally embed in the README below the GIF. Takes
~10 minutes.

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
