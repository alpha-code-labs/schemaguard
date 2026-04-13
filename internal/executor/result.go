package executor

import "time"

// StatementResult captures the outcome of a single migration statement.
// It is the atomic unit of per-statement timing that downstream M3+
// lock analysis and M5+ reporting will consume.
type StatementResult struct {
	// Index is the 0-based position of the statement in the original
	// migration script after splitting.
	Index int

	// SQL is the trimmed source text of the statement as it was
	// executed (without the trailing semicolon).
	SQL string

	// StartedAt is the wall-clock time immediately before Exec was
	// called for this statement. It is captured on every statement —
	// successful or failing — so the M3 lock analyzer can attribute
	// pg_locks samples to whichever statement window was executing
	// when each sample was taken.
	StartedAt time.Time

	// Duration is the wall-clock time spent in Exec for this statement.
	// It is captured even when the statement errored out.
	Duration time.Duration

	// Err is nil on success and non-nil on failure. The executor stops
	// after the first non-nil Err per docs/DECISIONS.md.
	Err error
}

// EndAt reports the wall-clock time at which the statement finished
// executing. It is derived from StartedAt + Duration and is the right
// edge of the statement window used for lock-sample attribution.
func (s StatementResult) EndAt() time.Time { return s.StartedAt.Add(s.Duration) }

// Result is the structured outcome of a single migration run.
//
// Milestone 2 is the first milestone to populate a Result. Later
// milestones read it:
//
//   - M3 (lock analyzer) attaches lock findings to whichever statements
//     were executing when the locks were taken, using Statements[i].
//   - M4 (plan regression analyzer) consults Failed to decide whether
//     post-migration plan capture runs at all. Per DECISIONS.md it is
//     skipped entirely when Failed is true.
//   - M5 (report generator) surfaces the migration-failure finding in
//     the "Migration Execution" section when Failed is true.
type Result struct {
	// Statements is the per-statement log in execution order. On a
	// failed run the failing statement is the last entry.
	Statements []StatementResult

	// ExplicitTx is true when the migration file contained its own
	// transaction-control statements (BEGIN/COMMIT/etc.) and the
	// executor therefore did NOT wrap the migration in an implicit
	// transaction.
	ExplicitTx bool

	// Failed is true when the executor halted because a statement
	// returned an error or a commit/rollback failed. Downstream stages
	// use this flag to decide whether post-migration analysis runs.
	Failed bool

	// FailureIndex is the Index of the statement that caused the run to
	// halt. It is -1 when Failed is false, or when the failure happened
	// outside of statement execution (e.g. during commit).
	FailureIndex int

	// FailureErr carries the underlying error that caused the failure.
	// It is nil when Failed is false.
	FailureErr error

	// TotalDuration is the total wall-clock time from the start of
	// execution to the end (success or failure). It does not include
	// snapshot restore time.
	TotalDuration time.Duration
}

// OK reports whether the migration ran to completion.
func (r *Result) OK() bool { return !r.Failed }
