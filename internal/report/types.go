package report

import "time"

// Verdict is the red/yellow/green classification of a whole run.
// It is a string type so JSON consumers get stable, self-describing
// values with no custom MarshalJSON plumbing.
type Verdict string

const (
	VerdictGreen  Verdict = "green"
	VerdictYellow Verdict = "yellow"
	VerdictRed    Verdict = "red"
)

// Rank maps a Verdict to an integer so callers can pick the most
// severe of several verdicts via simple max. Higher is worse.
func (v Verdict) Rank() int {
	switch v {
	case VerdictGreen:
		return 0
	case VerdictYellow:
		return 1
	case VerdictRed:
		return 2
	}
	return 0
}

// Emoji returns the icon used for this verdict in text and Markdown
// output. The emoji set is stable across formatters.
func (v Verdict) Emoji() string {
	switch v {
	case VerdictGreen:
		return "🟢"
	case VerdictYellow:
		return "🟡"
	case VerdictRed:
		return "🔴"
	}
	return "⚪"
}

// Label returns the short human label for the verdict.
func (v Verdict) Label() string {
	switch v {
	case VerdictGreen:
		return "Safe"
	case VerdictYellow:
		return "Caution"
	case VerdictRed:
		return "Stop"
	}
	return "Unknown"
}

// ExitCode maps a verdict to the process exit code defined in
// internal/cli/exitcode.go:
//
//	green  → 0
//	yellow → 1
//	red    → 2
//
// Tool-level errors (bad inputs, Docker unavailable, crash) are
// NOT represented in Verdict — they exit with code 3 from the CLI
// before this package is ever called.
func (v Verdict) ExitCode() int {
	switch v {
	case VerdictGreen:
		return 0
	case VerdictYellow:
		return 1
	case VerdictRed:
		return 2
	}
	return 2
}

// Severity is the per-finding classification. Like Verdict it is a
// string type for stable JSON serialization.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityCaution Severity = "caution"
	SeverityStop    Severity = "stop"
)

// Rank maps a Severity to an integer for max-severity computation.
// Higher is worse.
func (s Severity) Rank() int {
	switch s {
	case SeverityInfo:
		return 0
	case SeverityCaution:
		return 1
	case SeverityStop:
		return 2
	}
	return 0
}

// Group is a finding category. Groups appear in the report in the
// canonical order defined by CanonicalGroupOrder regardless of
// whether any findings exist in each group — empty groups are
// omitted by formatters.
type Group string

const (
	GroupMigrationExecution Group = "migration_execution"
	GroupLockRisk           Group = "lock_risk"
	GroupQueryPlan          Group = "query_plan_regressions"
)

// CanonicalGroupOrder is the display order formatters iterate in.
// Migration Execution comes first because a migration-level failure
// is always the most important fact in the report.
var CanonicalGroupOrder = []Group{
	GroupMigrationExecution,
	GroupLockRisk,
	GroupQueryPlan,
}

// Title returns the human label for a group, used in text and
// Markdown headings.
func (g Group) Title() string {
	switch g {
	case GroupMigrationExecution:
		return "Migration Execution"
	case GroupLockRisk:
		return "Lock Risk"
	case GroupQueryPlan:
		return "Query Plan Regressions"
	}
	return string(g)
}

// Finding is the normalized per-issue record that every formatter
// consumes. It deliberately flattens the shape of the M3 lock
// finding, the M4 plan finding, and the synthetic migration-
// failure finding into one small struct so there is exactly one
// rendering contract across the three formatters.
type Finding struct {
	Group    Group    `json:"group"`
	Severity Severity `json:"severity"`
	Kind     string   `json:"kind"`
	Object   string   `json:"object"`
	Impact   string   `json:"impact,omitempty"`
	Reason   string   `json:"reason"`
}

// Footer carries the run-level metadata that appears at the bottom
// of every report: tool version, timings, shadow DB image and size,
// and a docs link. Fields are optional; formatters omit zero values.
type Footer struct {
	ToolVersion       string        `json:"toolVersion,omitempty"`
	RunDuration       time.Duration `json:"runDurationMs,omitempty"`
	RestoreDuration   time.Duration `json:"restoreDurationMs,omitempty"`
	MigrationDuration time.Duration `json:"migrationDurationMs,omitempty"`
	ShadowDBImage     string        `json:"shadowDbImage,omitempty"`
	ShadowDBSizeBytes int64         `json:"shadowDbSizeBytes,omitempty"`
	DocsURL           string        `json:"docsUrl,omitempty"`
}

// Report is the single in-memory structure that every formatter
// consumes. SchemaVersion is the top-level versioning tag for JSON
// consumers — any incompatible change to the JSON layout must bump
// it.
type Report struct {
	SchemaVersion string    `json:"schemaVersion"`
	Verdict       Verdict   `json:"verdict"`
	Summary       string    `json:"summary"`
	Findings      []Finding `json:"findings"`
	Footer        Footer    `json:"footer"`
}

// FindingsByGroup returns the subset of Findings whose Group
// matches g. The returned slice preserves the order from the
// underlying Findings slice (which the builder sorts severity-
// desc, group-order, object-asc).
func (r *Report) FindingsByGroup(g Group) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Group == g {
			out = append(out, f)
		}
	}
	return out
}
