// Package planregression captures plan-only EXPLAIN (FORMAT JSON)
// plans for a set of user-specified top queries before and after a
// migration, then diffs the two plan sets and produces regression
// findings.
//
// The package is the M4 deliverable (see docs/tasks.md). Per
// docs/DECISIONS.md the analyzer uses plan-only EXPLAIN — never
// EXPLAIN ANALYZE — and never executes the top queries. The diff
// logic is deterministic and small: everything a Finding claims can
// be reproduced from the raw plan JSON alone.
//
// Like internal/lockanalyzer, this package does NOT render human-
// readable output. Formatting is the caller's concern; the temporary
// M2+ plumbing in internal/cli/check.go renders findings today, and
// the M5 report generator will consume them later from the same
// Finding type.
package planregression
