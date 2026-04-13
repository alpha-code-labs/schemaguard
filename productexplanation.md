# SchemaGuard — explained simply

---

## 1. Who this product is for

SchemaGuard is built for the **platform engineers and infrastructure engineers** inside software companies that run Postgres databases in production.

These are the people responsible for keeping the company's database running smoothly. They sit between the product teams (who want to ship features fast) and the production systems (which must not go down). When a developer wants to change how data is stored — add a new column, rename a field, create a new index — it is these engineers who worry about whether that change will break something for real customers.

They would care about SchemaGuard because they have almost certainly lived through this experience: a change that looked completely harmless in code review caused an outage in production. The database locked up, customer requests failed, and they spent their weekend fixing it. They don't want that to happen again, and today they have no reliable way to catch these problems before they reach production.

---

## 2. The problem it solves

Software companies constantly need to change how their databases are structured. A new feature might require adding a column. A redesign might rename a field. A performance improvement might need a new index. These changes are called **migrations** — small scripts that modify the database's structure.

The problem is that **migrations that look safe in code are often dangerous against real data.**

A one-line change like "add a column with a default value" might look trivial. But when that change runs against a table with 50 million customer records, it can silently lock the entire table for 15 minutes. During that time, no one can read from or write to that table. Orders fail. The website goes down. Support gets flooded.

Today, engineers have no standard way to test a migration against realistic data before deploying it. They review the SQL in a code review, where it looks fine. They run it against a tiny test database, where it finishes instantly. Then they deploy it to production, where it triggers a multi-minute lockup against millions of rows that no one predicted.

The result: outages, lost revenue, weekend emergencies, and a team that ships every migration with their fingers crossed.

---

## 3. How it solves the problem

SchemaGuard works like a **safety inspector for database changes**. Before the change goes to production, SchemaGuard runs it against a realistic copy of the company's data in a safe environment and reports exactly what would happen.

Here is the step-by-step flow:

1. **The engineer opens a pull request** containing a migration (a SQL file that changes the database structure).

2. **SchemaGuard automatically spins up a temporary test database** inside Docker on the CI server. This database is throwaway — it exists only for the duration of the test and is destroyed afterward.

3. **It restores a snapshot** of the company's real production data (a sanitized copy the team provides) into that temporary database.

4. **It runs the migration against the real-shaped data**, measuring what actually happens: How long does the database lock up? Which locks are taken? How do the most important queries perform before and after?

5. **It produces a clear report** — a simple red/yellow/green verdict posted as a comment directly on the pull request:
   - 🟢 **Green** — no problems found. Safe to merge.
   - 🟡 **Yellow** — some concerns. Review before merging.
   - 🔴 **Red** — serious problems. Do not merge.

6. **The engineer reads the comment**, sees exactly what went wrong (a lock that lasted 12 seconds, a query whose performance dropped 50x, a query that completely broke), and fixes the migration before it ever touches production.

**The before-vs-after story is simple:**
- **Before:** The engineer ships the migration, hopes for the best, and reacts to outages after the fact.
- **After:** The engineer sees the exact problems on the pull request, fixes them before merging, and ships with confidence.

---

## 4. What it is not

**It is not a dashboard or a monitoring system.** It does not watch your production database. It runs once per migration, in CI, as a check before you deploy.

**It is not a SaaS product or a hosted service.** It is an open-source command-line tool and a GitHub Action. It runs on your own CI servers. There is no account to create, no vendor to depend on, and no data leaving your infrastructure.

**It is not a static SQL linter.** Tools like Squawk and Atlas read your SQL and flag patterns that are known to be risky (like "don't use `NOT NULL` without a default"). SchemaGuard does something different: it actually runs the migration against real data and measures what happens. A static linter tells you what *might* go wrong based on the SQL text. SchemaGuard tells you what *will* go wrong based on observed behavior.

**It is not a replacement for your migration runner.** Tools like Flyway, Liquibase, and Alembic execute migrations — they are the plumbing that applies changes to your database in the right order. SchemaGuard does not replace them. It sits alongside them in your CI pipeline and verifies whether a migration is safe *before* your runner applies it to production.

**It is not an AI tool.** It does not use machine learning, LLMs, or heuristics that cannot be explained. Every finding it produces can be traced back to a concrete lock observation or a concrete query-plan change in the temporary database.

---

## 5. Why it is different

There are three layers of tools that exist today, and each covers a different part of the migration safety problem. SchemaGuard fills the gap between the other two:

**Migration runners** (Flyway, Liquibase, Alembic) are like the delivery truck. They take your migration script and execute it against the database in the right order. They answer the question: "How do I run this change?" They do not answer the question: "Should I run this change?"

**Static linters** (Squawk, Atlas) are like a code reviewer who reads the SQL. They flag patterns that are known to be risky — like adding a column without a default, or creating an index without the `CONCURRENTLY` keyword. They are fast and cheap, but they are blind to anything that depends on the actual data. They cannot tell you that your 50-million-row table will lock for 12 minutes, because they never touch the data.

**SchemaGuard** is like a dress rehearsal. It actually runs the migration against a realistic copy of your data, in a temporary database, and measures the real consequences — lock durations, query performance changes, queries that break. It catches problems that static linters cannot see because those problems only emerge when SQL meets real data at scale.

The three are complementary. Use a linter to catch the obvious syntax-level risks. Use SchemaGuard to catch the data-dependent risks. Use your migration runner to apply the changes you are confident about.

---

## 6. Three simple examples

### Example 1: The invisible table lock

**What the engineer thought was happening:**
"I'm just adding a new column to the orders table with a default value. It's one line of SQL. Should take a fraction of a second."

**What actually could have gone wrong:**
On Postgres versions before 11, or when the default involves a computation (like generating a random value per row), adding a column with a default rewrites every row in the table. On a 10-million-row orders table, this locks the entire table for 3–10 minutes. During that time, no customer can place an order, view their order history, or do anything that touches the orders table. The website effectively goes down.

**How SchemaGuard would have helped:**
SchemaGuard runs the migration against the seeded snapshot, observes the `AccessExclusiveLock` held on the orders table for multiple seconds, flags it as a "probable full table rewrite," and posts a 🔴 Red verdict on the pull request with the exact lock duration. The engineer sees the problem before merging and rewrites the migration to avoid the rewrite.

### Example 2: The index that blocks writes

**What the engineer thought was happening:**
"I'm creating a new index on the `created_at` column so our reports run faster. Creating an index is a harmless operation."

**What actually could have gone wrong:**
A regular `CREATE INDEX` (without the `CONCURRENTLY` keyword) takes a `ShareLock` on the table for the entire duration of the index build. On a million-row table, the build takes 1–3 seconds. During that time, every `INSERT`, `UPDATE`, and `DELETE` on the table is blocked. If the application sends 500 writes per second, those 1,500 writes all queue up and either time out or slam the database the moment the lock releases. Users see errors, and the operations team gets paged.

**How SchemaGuard would have helped:**
SchemaGuard runs the `CREATE INDEX`, observes the `ShareLock` held for 1.4 seconds, classifies it as 🟡 Caution with the note "blocks writes," and posts the finding on the pull request. The engineer adds `CONCURRENTLY` to the index creation, which avoids the blocking lock entirely.

### Example 3: The renamed column that breaks queries

**What the engineer thought was happening:**
"I'm renaming `customer_id` to `user_id` to match our new naming convention. It's a metadata change — instant and free."

**What actually could have gone wrong:**
The rename is indeed instant from the database's perspective. But every existing query in the application that references `customer_id` — every `SELECT`, every `WHERE` clause, every report — now fails with "column customer_id does not exist." If the application has even one query that was not updated to use the new name, it breaks in production the moment the migration runs. The failure is silent until a customer hits the affected page and gets an error.

**How SchemaGuard would have helped:**
SchemaGuard captures the query plans for the team's most important queries before and after the migration. When the post-migration `EXPLAIN` for the query `SELECT ... WHERE customer_id = 42` fails with "column does not exist," SchemaGuard flags it as 🔴 Red / "query broken" and shows the exact Postgres error on the pull request. The engineer sees that the migration will break a live query and coordinates the rename with the application code change before merging.

---

## 7. Simple short explanations

### 2-sentence explanation

SchemaGuard runs your database migration against a copy of your real data before you deploy it, and tells you if it would cause problems like table locks, slow queries, or broken code. It posts the results as a comment on your pull request so you can fix the problems before they reach production.

### 30-second explanation

When software teams change their database structure, the change often looks fine in code review but causes outages when it hits production — because the real impact depends on the actual data, not the SQL text. SchemaGuard is a free, open-source tool that runs your migration against a realistic copy of your data inside a temporary test database, measures what actually happens (lock durations, query performance changes, broken queries), and posts a simple red/yellow/green verdict on your pull request. You fix the problem before merging, instead of reacting to an outage after deploying.

### 1-minute explanation

Every software company that runs Postgres has the same recurring problem: database migrations that look harmless in code review cause outages in production. A one-line column addition locks the table for 10 minutes. An index creation blocks all writes for 3 seconds. A column rename silently breaks a query that 10,000 customers hit every hour. These problems cannot be caught by reading the SQL — they only appear when the SQL runs against real data at real scale.

SchemaGuard is an open-source tool that catches these problems before deployment. It sits in your CI pipeline as a GitHub Action. When an engineer opens a pull request with a migration, SchemaGuard automatically spins up a temporary Postgres database, loads a copy of your production data into it, runs the migration, and measures what actually happens. It checks for dangerous locks, query performance regressions, and broken queries — then posts a clear red/yellow/green report as a comment on the pull request.

The engineer sees the exact problems, fixes the migration, and merges with confidence. No outage. No weekend emergency. No crossed fingers. The tool costs nothing (it is MIT-licensed open source), runs entirely on your own infrastructure (no data leaves your systems), and complements existing tools like Flyway (which runs migrations) and Squawk (which lints SQL). Those tools tell you what your SQL looks like. SchemaGuard tells you what your migration actually does.
