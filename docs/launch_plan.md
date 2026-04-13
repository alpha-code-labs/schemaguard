# Launch Plan — Schema Migration Verification (Postgres-first OSS)

## One-line positioning

**"Runs your Postgres migration against real data in CI and tells you what will break."**

Alternate working phrasings for different surfaces:
- GitHub repo tagline: `Catch unsafe Postgres migrations in CI — by actually running them.`
- Show HN title: `Show HN: [Name] – Runs your Postgres migration in CI and reports what will break`
- One-sentence pitch in conversation: *"Static linters tell you what might go wrong. We run your migration against a shadow copy of your real data and tell you what will — locks, query plan regressions, broken dbt models. PR comment in 60 seconds."*

---

## Core launch message

**Pain to emphasize:** the specific, recognizable moment a platform engineer has lived through — "we shipped a migration Tuesday night, it locked the orders table for 14 minutes, support got flooded, I spent the weekend on it." Every paragraph of your messaging should point back to that moment.

**What to avoid completely:**
- "Database observability" / "shift left" / "reliability platform" / "AI-powered" / "intelligent"
- Any comparison to Datafold ("Datafold for X") — makes you look derivative
- Any mention of "enterprise," "compliance," "governance" — not your buyer yet
- Hype about AI coding tools generating your migrations faster — wrong audience
- Roadmap promises (multi-DB, dashboards, SaaS). Ship the wedge.

---

## Pre-launch checklist (all of this must be done before Day 1)

**Demo repo — the most important asset**
- Public GitHub repo: `[name]-demo` (e.g., `schemaguard-demo`)
- Realistic e-commerce schema: `users`, `orders`, `products`, `order_items`
- Seeded with enough data to make lock timings real (~1M rows in orders)
- 5 pre-made migration PRs, each triggering a distinct failure:
  1. `ADD COLUMN NOT NULL DEFAULT` → full table rewrite
  2. `CREATE INDEX` without `CONCURRENTLY` → write lock
  3. `DROP COLUMN` that a dbt model references → downstream break
  4. `RENAME COLUMN` that a top query references → query plan regression
  5. `ADD CHECK CONSTRAINT` → full table scan during validation
- README at the top of the demo repo: "clone this, run one command, see 5 real problems get caught in 60 seconds"

**Main repo README — written in this order**
1. Headline + tagline
2. Animated GIF of the PR comment appearing (this is the hook)
3. 30-second quickstart (3 commands max)
4. Philosophy: "Static linters check what your SQL looks like. We check what your migration actually does." (2 short paragraphs)
5. Honest comparison paragraph: "Complements Squawk (static) and Atlas (migration lint). Use all three if you want." — this earns trust instead of spending it
6. What's supported today (Postgres only), what's explicitly not
7. Install, usage, GitHub Action example
8. Contributing, license, roadmap (kept brutally short)

**Landing page — DO NOT build one in week 1.** The GitHub repo *is* your landing page for this audience. A marketing site makes you look less credible to platform engineers, not more. Build one in month 2 if you need it for outbound.

**Demo video / GIF**
- 90 seconds max, no voice required
- Screen recording: open a PR with a bad migration → GitHub Action runs → red PR comment appears → engineer reads it → closes PR
- Embedded in README as a GIF and uploaded to YouTube unlisted for sharing

**Screenshots**
- One hero screenshot of a PR comment with 3 findings (green, yellow, red)
- One terminal screenshot showing the CLI output
- That's all you need

**GitHub Action Marketplace listing**
- Name, description, icon, one-line how-to
- Points to the main repo

**Show HN post — written and edited a week before launch**
- Title (see above)
- First comment drafted: who you are (1 sentence, no credentials padding), why you built it, what's different from Squawk/Atlas/Datafold in one honest sentence each, what feedback you want, thanks.

**Short technical blog post (high-leverage, optional but recommended)**
- "We analyzed 200 recent migration commits from 50 popular open-source Postgres projects. Here's what we found." Use your own tool to generate the data. This post proves the tool works and gives you something to share that isn't a product pitch. Publish day of launch.

---

## Day 1 launch plan

**Pick a Tuesday or Wednesday.** Avoid Mondays (cleaning inboxes), Fridays (nobody reads HN).

**6:00–7:30 AM Pacific:**
- Final check: demo repo clones cleanly on a fresh machine, every install command works, GIF loads, Action posts a comment end-to-end.

**7:30 AM Pacific:**
- Post to Hacker News as `Show HN: [Name] – [one-line description]`. Don't submit from the company Twitter or any alt account — submit from your main HN account.
- Immediately post your prepared first comment explaining context.

**7:30 AM – 11:00 PM Pacific (yes, all day):**
- **Refresh HN every 3–5 minutes.** Reply to every single comment within 10 minutes. This is not optional. HN rewards engagement velocity.
- Thank critics genuinely. When someone says "isn't this just Datafold?" reply with a specific, short, honest answer — not a defensive wall of text.
- Never argue. Never say "we already handle that" unless you actually do. Say "that's a fair point, we don't handle that yet — how does your team handle it today?" Turn every critic into an informant.

**Do NOT on Day 1:**
- Cross-post to Reddit, Lobsters, or Slack communities. HN mods penalize this. Wait 24 hours.
- Post to Product Hunt. Wrong audience entirely.
- DM 50 strangers on LinkedIn.
- Tweet about it if you have no audience. Pointless.
- Ask friends to upvote. HN detects voting rings and you will be buried permanently.

**Late evening Day 1:**
- Write a short thank-you update comment on HN summarizing interesting feedback.
- File GitHub issues for every substantive piece of feedback.

---

## Week 1 plan (Days 2–7)

**Day 2:**
- Post to `r/PostgreSQL` with a different framing: "I built an OSS tool that catches unsafe Postgres migrations in CI — would love feedback from folks running real workloads." Link the demo repo, not the HN post.
- Post to `lobste.rs` if someone can invite you.
- Respond to every new GitHub issue and star.

**Day 3:**
- Post to the `#tools` or equivalent channel in Platform Engineering Slack, Data Engineering Slack, dbt Slack, Locally Optimistic. One post each, not a copy-paste — tailor each one slightly to the community.
- DO NOT post the same text in 10 Slacks on the same day. It reads as spam and burns you with the exact audience you need.

**Day 4:**
- Publish the "200 migrations analyzed" technical post on your own blog or dev.to. Link it on HN as a follow-up comment, submit it as a separate HN post in a few days if Day 1 did well.
- Email the maintainers of Squawk and Atlas as a courtesy with a short note: "I built this, it's complementary to yours, I linked you in the README, would love feedback when you have time." This is how you enter the community without stepping on toes — and they often share tools they respect.

**Day 5:**
- First pass at identifying power users: who starred + cloned + opened an issue + commented with a real use case? Make a spreadsheet. You should have 10–30 names by Day 5 if the launch worked.

**Day 6–7:**
- Reach out to 10 of those power users with a personal message:
  > "Thanks for trying [tool]. I saw you [specific action they took]. I'm the builder — would love 20 minutes to hear how you're using it and what's missing. No pitch, no sales. I just want to make this useful. Here are two slots this week: [links]."

---

## Weeks 2–4 plan

**Core rhythm:**
- **Every morning:** respond to every issue, PR, and mention within 2 hours. This is your single biggest differentiator as an unknown founder. OSS maintainers who reply fast get trust that funded companies can't buy.
- **Every day:** ship one small, visible improvement. A new failure-mode check, a better error message, a bug fix. Post it as a release note. Shipping velocity is a signal to early users that this project is alive.
- **3x per week:** targeted outbound to 5 platform leads at specific companies. Criteria: public engineering blog mentioning Postgres scale, recent job postings for DBRE/platform roles, visible on GitHub using Alembic/Flyway. Message: specific observation about their stack + demo link + "would this have caught [specific type of failure]?"

**Content:**
- Week 2: Follow-up post — "What 200 migrations taught us about unsafe patterns." Use it as outbound material.
- Week 3: Write one comparison post that is NOT about competitors — about patterns. "5 Postgres migration patterns that look safe and aren't." Link tool at the bottom.
- Week 4: Try to get into Platform Weekly, DevOps'ish, Last Week in AWS (Postgres section), Postgres Weekly. These newsletters are the real distribution channels for OSS dev infra.

**Conversations:**
- Target: 3 design-partner conversations per week in weeks 2–4.
- Structure: 25 minutes, no slides, start with "tell me about the last migration that scared you." Shut up and listen for 20 minutes. Last 5 minutes: "would you be open to using this on real migrations with me helping hands-on for 2 weeks, free?"

---

## Days 31–60 plan

**Goal of this phase:** convert the attention from launch into 2–3 committed design partners and a clear signal on what to charge for.

**Design partner pilots:**
- Week 5–6: Onboard first 2 design partners. Hands-on. Join their Slack. Be embarrassingly responsive. Set up the tool on their repo, help them integrate it, sit next to them (virtually) during their first real migration run.
- Week 7–8: Weekly 30-min feedback calls with each. Ship what they ask for — but only if it generalizes. If a request is truly bespoke, write it in `v2.md` and move on.

**Content:**
- Day 45: "What we learned from 30 days of a launching a Postgres migration safety tool" post. Lessons, numbers, failure modes you've now seen across real codebases. This is the post that drives the second wave of users and makes you look legitimate.
- Day 60: Publish a short "patterns we've seen across design partners" post (anonymized). Earns credibility without being a sales pitch.

**Commercial exploration (don't rush it):**
- Day 50–60: in design partner calls, start asking "how would your team pay for this if we offered a paid tier?" Don't propose pricing. Listen to what they say.
- By Day 60: you should have a rough sense of whether this is a $10K, $30K, or $100K ACV tool and what gated feature unlocks the paid tier.

**Do NOT in days 31–60:**
- Build a SaaS version yet
- Hire
- Take investor meetings (one or two exploratory ones max, nothing serious)
- Expand to MySQL or another database
- Redesign the landing page
- Run paid ads

---

## Design partner strategy

**Who to target:**
- Company stage: Series B/C, 100–500 engineers
- Stack: Postgres 1–50TB, Alembic or Flyway, dbt downstream, GitHub or GitLab PRs
- Signal: public engineering blog with at least one post mentioning a migration-related incident, or job posts for DBRE/platform eng
- Industry: fintech, SaaS, health-tech, e-commerce — any business where a 12-minute table lock is a named disaster

**Who exactly to approach:**
- Title: Staff/Principal Engineer on the Platform team, or DBRE, or Head of Infrastructure
- Not: CTO (too high), junior engineers (no budget), VP Eng (too abstract), Security (wrong framing)

**First-pilot structure:**
- 6 weeks total
- Week 1: hands-on setup, you do 80% of the work — get the tool running on their CI against a shadow DB from a masked prod snapshot
- Weeks 2–5: run on every real migration PR. 30-minute weekly feedback call.
- Week 6: retrospective call — what caught real problems, what was noise, what do they wish existed

**What to offer free:**
- The tool itself, for 6 months
- Hands-on setup and support
- Name on the "design partners" section of the README (with permission)
- Early access to every new feature
- Direct Slack access to you

**What to ask in return:**
- 30-min weekly feedback call
- Permission to quote them (anonymized is fine)
- An intro to one similar company if it works
- Honest feedback including negative — you'd rather hear "this isn't helpful" than polite silence

**What NOT to do:**
- Don't charge them
- Don't take investment from them
- Don't offer equity or revenue share
- Don't sign a contract longer than one page
- Don't commit to a roadmap

---

## Metrics to track

**Track religiously (Days 1–30):**
- GitHub stars (crude signal but fast)
- Unique clones per week (via GitHub insights — real usage signal)
- GitHub Action runs (if you add opt-in telemetry; otherwise skip)
- Issues opened (high-value signal — especially ones with real migration SQL in them)
- Demo repo forks/clones (means they went past looking)
- Number of substantive conversations with platform engineers (not just lurkers)
- Inbound "can I try this on my real repo?" messages
- Design partner commitments

**Track secondarily (Days 31–60):**
- Weekly active repos using the Action (if telemetry exists)
- Design partner retention (are they still running it weekly?)
- Pilot-to-paid interest signal (who asks about pricing first?)
- Newsletter mentions, podcast invites (light indicators of reach)

**Vanity metrics to explicitly ignore:**
- Twitter/X followers
- LinkedIn impressions
- Product Hunt upvotes
- Stars-without-engagement (1000 stars and 0 issues = nothing)
- Newsletter signups on a marketing site you shouldn't have built
- Press mentions in tier-3 outlets

**Realistic good numbers by Day 30:**
- 1,500–3,000 stars
- 20–80 unique installs / Action usages
- 10–25 substantive issues
- 5–15 real conversations with platform engineers
- 1–3 design partner commitments

**Realistic good numbers by Day 60:**
- 3,500–6,000 stars
- 2–4 active design partners
- Clear signal on what feature unlocks paid tier
- 1–2 newsletter or podcast mentions
- Rough pricing hypothesis

---

## Biggest mistakes to avoid

1. **Launching before the demo repo is rock-solid.** One broken install command on Day 1 kills the launch. Test on a clean VM.
2. **Cross-posting to every channel on Day 1.** HN flags it, communities flag you as a spammer, and you burn your best shot. Space posts by 24–48 hours.
3. **Being defensive in Show HN comments.** The people criticizing you on HN are your best informants. Every defensive reply costs you a potential user.
4. **Treating stars as customers.** Stars are an attention metric. A single 30-minute call with a real platform lead is worth 500 stars.
5. **Building a marketing website with fake testimonials or "Trusted by" logos.** This audience can smell it and it ends your credibility permanently.
6. **Framing yourself against Datafold or Atlas.** You look small, derivative, and forgettable. Frame against the pain, not the competitor.
7. **Writing "roadmap" posts instead of shipping.** Ship daily. Roadmap posts are what people do when the product isn't good enough to talk about on its own.
8. **Hiring, fundraising, or launching paid tiers in the first 60 days.** All three kill focus at the exact moment you need to be listening to users.
9. **Ignoring the dbt / downstream angle.** It's a huge part of why your tool is different. Lean on it.
10. **Being inaccessible.** As an unknown founder, "the maintainer replied to my issue in 10 minutes" is your single biggest unfair advantage. Lose it and you become every other abandoned OSS project.
11. **Adding MySQL support to look bigger.** You will half-ship both. Stay Postgres-only until Postgres is winning.
12. **Obsessing over Twitter/X/LinkedIn with no audience.** Zero leverage. Spend that time on GitHub issues instead.

---

## Final verdict — what a realistic good launch looks like

A realistic good outcome at Day 60 is **not** "we're the hot OSS project of the month." It's this:

- Around 4,000 GitHub stars, with 50–100 real users who run the tool on real migrations every week
- 2–3 design partners who will get on a call when you need feedback, and who'd pay something if you asked
- One technical blog post that platform engineers forward to each other
- A short list of 5–10 known-name companies using the tool in CI
- One newsletter mention and maybe one podcast invite
- A clear, evidence-based hypothesis about which feature unlocks the first paid tier
- You personally know 15–25 platform engineers by name and can DM them when you ship something new

That's it. That's the goal. It looks small on the outside. On the inside, it's a fully loaded catapult for month 3 onward — when you start converting those design partners into paying customers and using their names to win the next 10.

**The trap to avoid above all else:** mistaking a successful HN launch for a successful company launch. The Show HN spike is the starting line, not the finish. The founders who win at this stage are the ones who take the attention and patiently, daily, turn it into a handful of real relationships over 60 days — while most others are refreshing star counts and writing Medium posts about their launch.

Build the tool. Ship the demo. Post it honestly. Reply fast. Talk to every user. Everything else is noise.
