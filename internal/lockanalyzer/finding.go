package lockanalyzer

import "time"

// Severity categorizes how dangerous a lock finding is for a real
// production deployment. The analyzer assigns Severity from first-pass
// defaults recorded in docs/DECISIONS.md ("Lock finding severity
// defaults") and M4+ pilots will tune them.
type Severity int

const (
	// SeverityInfo — an observed lock that does not meaningfully
	// threaten concurrent reads or writes in production.
	SeverityInfo Severity = iota

	// SeverityCaution — a lock that would degrade concurrent access
	// under production load but is unlikely to cause an outage.
	// Corresponds to the yellow verdict level in the eventual M5
	// report.
	SeverityCaution

	// SeverityStop — a lock that would likely cause a real outage in
	// production (e.g. a long AccessExclusive hold during peak
	// traffic). Corresponds to the red verdict level.
	SeverityStop
)

// String returns a short label for the severity, used by the M3
// temporary CLI plumbing output in internal/cli/check.go. The eventual
// M5 report generator renders Severity its own way and does not depend
// on this string.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityCaution:
		return "caution"
	case SeverityStop:
		return "stop"
	}
	return "unknown"
}

// FindingKind distinguishes a generic lock observation from the
// narrower "probable full-table rewrite" classification described in
// tasks.md 3.4.
type FindingKind int

const (
	// KindLock — a generic relation-level lock observation.
	KindLock FindingKind = iota

	// KindTableRewrite — an observed AccessExclusive lock held on a
	// user relation for most of a statement that took non-trivial
	// wall-clock time. This is the observed-behavior signal for a
	// probable full-table rewrite.
	KindTableRewrite
)

// Finding is the structured output of the analyzer for one observed
// lock event. Durations are measured from the sample stream — they
// are a lower bound on the true lock lifetime because the sampler is
// discrete.
type Finding struct {
	Kind FindingKind

	// StatementIndex is the 0-based index of the statement the lock
	// was attributed to. -1 means attribution could not pin the lock
	// to any specific statement window (e.g. lock predated the first
	// sample).
	StatementIndex int

	// Object is the schema-qualified relation name as reported by
	// Postgres (e.g. "public.users").
	Object string

	// Mode is the Postgres lock mode as reported by pg_locks
	// (e.g. "AccessExclusiveLock").
	Mode string

	// Granted is true if every sample that observed the lock had
	// granted = true. A false value here means the migration was
	// blocked waiting for the lock at least once.
	Granted bool

	// Duration is the observed time between the first and last sample
	// that carried this (object, mode) pair, plus one sampling interval
	// as a small padding so single-sample observations have non-zero
	// duration.
	Duration time.Duration

	// BlocksReads reports whether the Mode conflicts with a normal
	// SELECT (AccessShare) in production.
	BlocksReads bool

	// BlocksWrites reports whether the Mode conflicts with a normal
	// INSERT/UPDATE/DELETE (RowExclusive) in production.
	BlocksWrites bool

	// Severity is the classification assigned by the analyzer based on
	// Mode and Duration.
	Severity Severity

	// Reason is a short human-readable explanation the M3 plumbing
	// output prints. It is not the final report surface.
	Reason string
}

// modeBlocksReads reports whether a lock mode conflicts with a
// concurrent AccessShare lock held by a SELECT. The Postgres lock
// conflict matrix is the source of truth; this is a direct encoding.
func modeBlocksReads(mode string) bool {
	switch mode {
	case "AccessExclusiveLock":
		return true
	}
	return false
}

// modeBlocksWrites reports whether a lock mode conflicts with a
// concurrent RowExclusive lock held by a DML statement.
func modeBlocksWrites(mode string) bool {
	switch mode {
	case "AccessExclusiveLock",
		"ExclusiveLock",
		"ShareRowExclusiveLock",
		"ShareLock":
		return true
	}
	return false
}

// classifySeverity assigns a first-pass severity based on the lock
// mode and observed duration. The thresholds are committed in
// docs/DECISIONS.md and documented per mode below.
//
// First-pass rules (M3):
//
//	AccessExclusive    < 100ms → info,   100–500ms → caution,  >500ms → stop
//	Exclusive / SREX   < 200ms → info,   200ms–1s  → caution,  >1s    → stop
//	Share / SUE        < 1s    → info,   1s–5s     → caution,  >5s    → stop
//	Everything else                                          always info
func classifySeverity(mode string, dur time.Duration) Severity {
	switch mode {
	case "AccessExclusiveLock":
		switch {
		case dur > 500*time.Millisecond:
			return SeverityStop
		case dur > 100*time.Millisecond:
			return SeverityCaution
		}
		return SeverityInfo
	case "ExclusiveLock", "ShareRowExclusiveLock":
		switch {
		case dur > 1*time.Second:
			return SeverityStop
		case dur > 200*time.Millisecond:
			return SeverityCaution
		}
		return SeverityInfo
	case "ShareLock", "ShareUpdateExclusiveLock":
		switch {
		case dur > 5*time.Second:
			return SeverityStop
		case dur > 1*time.Second:
			return SeverityCaution
		}
		return SeverityInfo
	}
	return SeverityInfo
}
