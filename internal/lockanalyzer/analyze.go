package lockanalyzer

import (
	"fmt"
	"sort"
	"time"
)

// StatementWindow describes one migration statement's execution
// window. It is the minimum input the analyzer needs — the caller
// populates it from executor.StatementResult without the analyzer
// having to import the executor package.
type StatementWindow struct {
	// Index is the 0-based statement index in the migration.
	Index int

	// StartedAt is the wall-clock start of the statement's Exec call.
	StartedAt time.Time

	// EndAt is the wall-clock end of the statement's Exec call
	// (StartedAt + Duration in executor.StatementResult).
	EndAt time.Time

	// SQL is the trimmed statement text. The analyzer does not parse
	// it — it is carried only so findings can reference the statement
	// in human-readable form.
	SQL string
}

// Duration reports the width of the statement window.
func (w StatementWindow) Duration() time.Duration { return w.EndAt.Sub(w.StartedAt) }

// Analyze groups a raw Sample stream into one Finding per distinct
// (object, mode) pair, attributes each finding to the statement whose
// window contains the first sample of that pair, classifies severity,
// and flags probable full-table rewrites.
//
// interval is the sampling interval the Sampler used, and is used as
// padding when a lock appears in only one sample so its duration is
// not zero.
//
// The returned findings are sorted in descending order of severity
// and then ascending by statement index so the most dangerous are
// visible first in the temporary M3 CLI output. The M5 report
// generator can re-sort however it likes.
func Analyze(samples []Sample, windows []StatementWindow, interval time.Duration) []Finding {
	if len(samples) == 0 {
		return nil
	}
	if interval <= 0 {
		interval = DefaultSamplingInterval
	}

	type key struct {
		Object string
		Mode   string
	}
	type agg struct {
		firstAt      time.Time
		lastAt       time.Time
		allGranted   bool
		sampleCount  int
	}

	groups := make(map[key]*agg)
	for _, s := range samples {
		k := key{Object: s.Object, Mode: s.Mode}
		g, ok := groups[k]
		if !ok {
			g = &agg{
				firstAt:    s.At,
				lastAt:     s.At,
				allGranted: true,
			}
			groups[k] = g
		}
		if s.At.Before(g.firstAt) {
			g.firstAt = s.At
		}
		if s.At.After(g.lastAt) {
			g.lastAt = s.At
		}
		if !s.Granted {
			g.allGranted = false
		}
		g.sampleCount++
	}

	findings := make([]Finding, 0, len(groups))
	for k, g := range groups {
		// Observed duration is last - first, plus one sampling
		// interval of padding so a lock seen in exactly one sample
		// still has a non-zero duration.
		dur := g.lastAt.Sub(g.firstAt) + interval

		// Attribute by maximum overlap between the lock's observed
		// lifetime and each statement window. Pad the lifetime by
		// half the sampling interval on each side so a lock observed
		// in a single sample still has non-zero width to overlap
		// with.
		lockStart := g.firstAt.Add(-interval / 2)
		lockEnd := g.lastAt.Add(interval / 2)
		idx := attributeToStatement(lockStart, lockEnd, windows)

		severity := classifySeverity(k.Mode, dur)

		f := Finding{
			Kind:           KindLock,
			StatementIndex: idx,
			Object:         k.Object,
			Mode:           k.Mode,
			Granted:        g.allGranted,
			Duration:       dur,
			BlocksReads:    modeBlocksReads(k.Mode),
			BlocksWrites:   modeBlocksWrites(k.Mode),
			Severity:       severity,
			Reason:         defaultReason(k.Mode, dur, idx),
		}

		if isLikelyTableRewrite(k.Mode, dur, idx, windows) {
			f.Kind = KindTableRewrite
			// A rewrite observation is never lower than caution. The
			// severity floor reflects that rewrites are almost always
			// bad news for production traffic.
			if f.Severity < SeverityCaution {
				f.Severity = SeverityCaution
			}
			f.Reason = fmt.Sprintf(
				"probable full table rewrite: %s held for %s (covers ~%d%% of statement #%d's %s)",
				k.Mode,
				dur.Round(time.Millisecond),
				percentOfWindow(dur, idx, windows),
				idx+1,
				statementDuration(idx, windows).Round(time.Millisecond),
			)
		}

		findings = append(findings, f)
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity > findings[j].Severity
		}
		if findings[i].StatementIndex != findings[j].StatementIndex {
			return findings[i].StatementIndex < findings[j].StatementIndex
		}
		return findings[i].Object < findings[j].Object
	})
	return findings
}

// attributeToStatement returns the 0-based index of the statement
// window that has the largest temporal overlap with the lock lifetime
// [lockStart, lockEnd]. If no window overlaps the lifetime at all, it
// returns -1.
//
// This heuristic matches tasks.md 3.2's "attribute by timestamp
// bracketing" while tolerating two real-world edge cases: (a) a lock
// that is first sampled a few milliseconds before the statement's
// Go-side StartedAt because of goroutine scheduling and wall-clock
// granularity, and (b) a lock whose samples straddle a statement
// boundary inside a single transaction. Ties go to the earliest
// statement with maximum overlap.
func attributeToStatement(lockStart, lockEnd time.Time, windows []StatementWindow) int {
	bestIdx := -1
	var bestOverlap time.Duration
	for _, w := range windows {
		start := lockStart
		if w.StartedAt.After(start) {
			start = w.StartedAt
		}
		end := lockEnd
		if w.EndAt.Before(end) {
			end = w.EndAt
		}
		if !end.After(start) {
			continue
		}
		overlap := end.Sub(start)
		if overlap > bestOverlap {
			bestOverlap = overlap
			bestIdx = w.Index
		}
	}
	return bestIdx
}

// isLikelyTableRewrite implements tasks.md 3.4: detect a full-table
// rewrite from *observed* behavior rather than from SQL pattern
// matching. The heuristic is intentionally simple:
//
//  1. The mode is AccessExclusive. Nothing weaker is a rewrite.
//  2. The attributed statement had a non-trivial wall-clock duration
//     (> 100ms). Metadata-only ALTER TABLE operations in PG 11+ are
//     much faster than this.
//  3. The observed lock duration covers at least 80% of the
//     statement window — i.e. the lock was held throughout the work,
//     not taken briefly at the end.
//
// Thresholds live in docs/DECISIONS.md and will be tuned with pilot
// data. The heuristic never looks at the SQL text.
func isLikelyTableRewrite(mode string, lockDur time.Duration, stmtIdx int, windows []StatementWindow) bool {
	if mode != "AccessExclusiveLock" {
		return false
	}
	if stmtIdx < 0 {
		return false
	}
	stmtDur := statementDuration(stmtIdx, windows)
	if stmtDur < 100*time.Millisecond {
		return false
	}
	if percentOfWindow(lockDur, stmtIdx, windows) < 80 {
		return false
	}
	return true
}

func statementDuration(idx int, windows []StatementWindow) time.Duration {
	for _, w := range windows {
		if w.Index == idx {
			return w.Duration()
		}
	}
	return 0
}

func percentOfWindow(lockDur time.Duration, stmtIdx int, windows []StatementWindow) int {
	d := statementDuration(stmtIdx, windows)
	if d <= 0 {
		return 0
	}
	pct := int((lockDur * 100) / d)
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}
	return pct
}

func defaultReason(mode string, dur time.Duration, stmtIdx int) string {
	if stmtIdx < 0 {
		return fmt.Sprintf("%s observed for %s (no statement window)", mode, dur.Round(time.Millisecond))
	}
	return fmt.Sprintf("%s observed for %s during statement #%d", mode, dur.Round(time.Millisecond), stmtIdx+1)
}
