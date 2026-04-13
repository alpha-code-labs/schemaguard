// Package executor applies migration SQL against the shadow database.
//
// Per docs/DECISIONS.md the executor wraps the full migration in a single
// transaction by default, respects explicit BEGIN/COMMIT if present in the
// migration file, and stops on the first SQL error. Lock findings produced
// before the failure (from M3 onward) are preserved; post-migration plan
// capture (M4 onward) is skipped on a failed run.
package executor
