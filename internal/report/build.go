package report

import (
	"fmt"
	"sort"
	"time"

	"github.com/alpha-code-labs/schemaguard/internal/executor"
	"github.com/alpha-code-labs/schemaguard/internal/lockanalyzer"
	"github.com/alpha-code-labs/schemaguard/internal/planregression"
)

// SchemaVersion is the top-level version stamp for JSON consumers.
// Any incompatible change to the JSON report layout MUST bump this
// value. Unit tests pin the current value so a silent edit cannot
// drift the schema.
const SchemaVersion = "1"

// DefaultDocsURL is the URL embedded in the report footer. It
// points at the repository root until a published docs site exists.
const DefaultDocsURL = "https://github.com/alpha-code-labs/schemaguard"

// Input is the set of per-run values the report builder consumes.
// Every field is optional in the sense that the builder will cope
// with a zero value, but a real run passes all of them.
type Input struct {
	ToolVersion       string
	MigrationResult   *executor.Result
	LockFindings      []lockanalyzer.Finding
	PlanFindings      []planregression.Finding
	RunDuration       time.Duration
	RestoreDuration   time.Duration
	ShadowDBImage     string
	ShadowDBSizeBytes int64
}

// Build consumes Input and returns the fully populated Report.
// Build is pure: no I/O, no side effects, no dependency on wall
// clock. It is safe to call from tests.
//
// Verdict rules (docs/tasks.md 5.2):
//
//   - A migration that halted on its own SQL error is always red
//     (exit 2), never tool error (exit 3). The synthetic migration-
//     execution finding drives this.
//   - Otherwise, the verdict is the max severity of any emitted
//     finding. Stop → red, Caution → yellow, Info → green.
//   - A run with no findings at all is green.
func Build(in Input) Report {
	var findings []Finding

	if in.MigrationResult != nil && in.MigrationResult.Failed {
		findings = append(findings, migrationFailureFinding(in.MigrationResult))
	}

	for _, f := range in.LockFindings {
		findings = append(findings, fromLockFinding(f))
	}

	// Per docs/DECISIONS.md and tasks.md 4.4, plan findings are only
	// produced when the migration succeeded. The builder still
	// accepts them defensively and drops them on failure so the
	// verdict rule "migration failure = red" always wins.
	if in.MigrationResult == nil || !in.MigrationResult.Failed {
		for _, f := range in.PlanFindings {
			findings = append(findings, fromPlanFinding(f))
		}
	}

	sortFindings(findings)

	verdict := computeVerdict(in.MigrationResult, findings)

	report := Report{
		SchemaVersion: SchemaVersion,
		Verdict:       verdict,
		Summary:       summaryFor(verdict, in.MigrationResult, findings),
		Findings:      findings,
		Footer: Footer{
			ToolVersion:       in.ToolVersion,
			RunDuration:       in.RunDuration,
			RestoreDuration:   in.RestoreDuration,
			MigrationDuration: migrationDurationOf(in.MigrationResult),
			ShadowDBImage:     in.ShadowDBImage,
			ShadowDBSizeBytes: in.ShadowDBSizeBytes,
			DocsURL:           DefaultDocsURL,
		},
	}
	return report
}

// computeVerdict implements the verdict rule from tasks.md 5.2.
// A failed migration short-circuits to red; otherwise the max
// severity among findings determines the verdict.
func computeVerdict(mig *executor.Result, findings []Finding) Verdict {
	if mig != nil && mig.Failed {
		return VerdictRed
	}
	worst := VerdictGreen
	for _, f := range findings {
		v := verdictForSeverity(f.Severity)
		if v.Rank() > worst.Rank() {
			worst = v
		}
	}
	return worst
}

func verdictForSeverity(s Severity) Verdict {
	switch s {
	case SeverityStop:
		return VerdictRed
	case SeverityCaution:
		return VerdictYellow
	}
	return VerdictGreen
}

// summaryFor returns a one-sentence plain-English summary of the
// run, varying by verdict and the findings that drove it.
func summaryFor(v Verdict, mig *executor.Result, findings []Finding) string {
	if mig != nil && mig.Failed {
		return "Migration halted on its own SQL error — do not merge."
	}
	stop := countAtSeverity(findings, SeverityStop)
	caution := countAtSeverity(findings, SeverityCaution)
	switch v {
	case VerdictRed:
		return fmt.Sprintf("Migration ran but raised %s; do not merge.", pluralFindings(stop, "stop-level"))
	case VerdictYellow:
		return fmt.Sprintf("Migration ran but raised %s; review before merging.", pluralFindings(caution, "caution-level"))
	}
	return "Migration ran cleanly — no significant findings."
}

func countAtSeverity(findings []Finding, s Severity) int {
	n := 0
	for _, f := range findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

func pluralFindings(n int, adj string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s finding", adj)
	}
	return fmt.Sprintf("%d %s findings", n, adj)
}

// migrationFailureFinding builds the synthetic MigrationExecution
// finding that represents a halted migration. The reason string
// includes the failing statement index (1-based) and the Postgres
// error the executor captured.
func migrationFailureFinding(r *executor.Result) Finding {
	obj := "migration"
	if r.FailureIndex >= 0 {
		obj = fmt.Sprintf("statement #%d", r.FailureIndex+1)
	}
	reason := "Migration halted on its own SQL error."
	if r.FailureErr != nil {
		reason = r.FailureErr.Error()
	}
	return Finding{
		Group:    GroupMigrationExecution,
		Severity: SeverityStop,
		Kind:     "migration_failure",
		Object:   obj,
		Impact:   "migration aborted, transaction rolled back",
		Reason:   reason,
	}
}

// fromLockFinding translates the lockanalyzer finding into a
// normalized report Finding. The Object field combines the Postgres
// lock mode and the affected relation so a single row in the text
// or Markdown output gives the reader the full "what and where."
func fromLockFinding(f lockanalyzer.Finding) Finding {
	kind := "lock"
	if f.Kind == lockanalyzer.KindTableRewrite {
		kind = "table_rewrite"
	}

	impact := fmt.Sprintf("held for %s", f.Duration.Round(time.Millisecond))
	blocking := ""
	switch {
	case f.BlocksReads && f.BlocksWrites:
		blocking = " — blocks reads + writes"
	case f.BlocksWrites:
		blocking = " — blocks writes"
	case f.BlocksReads:
		blocking = " — blocks reads"
	}
	if blocking != "" {
		impact += blocking
	}
	if !f.Granted {
		impact += " (waited)"
	}

	return Finding{
		Group:    GroupLockRisk,
		Severity: severityFromLock(f.Severity),
		Kind:     kind,
		Object:   fmt.Sprintf("%s on %s", f.Mode, f.Object),
		Impact:   impact,
		Reason:   f.Reason,
	}
}

// fromPlanFinding translates the planregression finding into a
// normalized report Finding. The Object field is the user's query
// id; Impact carries either the cost/rows numbers or the scan
// mode transition.
func fromPlanFinding(f planregression.Finding) Finding {
	kind := planKindString(f.Kind)
	impact := ""
	switch f.Kind {
	case planregression.KindCostIncrease:
		impact = fmt.Sprintf("cost %.0f → %.0f", f.BaselineCost, f.PostCost)
	case planregression.KindRowsIncrease:
		impact = fmt.Sprintf("rows %.0f → %.0f", f.BaselineRows, f.PostRows)
	case planregression.KindScanDowngrade, planregression.KindNewSeqScan:
		impact = fmt.Sprintf("%s → %s on %s", f.BaselineScan, f.PostScan, f.Relation)
	case planregression.KindQueryBroken, planregression.KindBaselineBroken:
		impact = f.ErrorMessage
	}
	return Finding{
		Group:    GroupQueryPlan,
		Severity: severityFromPlan(f.Severity),
		Kind:     kind,
		Object:   f.QueryID,
		Impact:   impact,
		Reason:   f.Reason,
	}
}

func planKindString(k planregression.FindingKind) string {
	switch k {
	case planregression.KindCostIncrease:
		return "cost_increase"
	case planregression.KindRowsIncrease:
		return "rows_increase"
	case planregression.KindScanDowngrade:
		return "scan_downgrade"
	case planregression.KindNewSeqScan:
		return "new_seq_scan"
	case planregression.KindQueryBroken:
		return "query_broken"
	case planregression.KindBaselineBroken:
		return "baseline_broken"
	}
	return "plan"
}

func severityFromLock(s lockanalyzer.Severity) Severity {
	switch s {
	case lockanalyzer.SeverityStop:
		return SeverityStop
	case lockanalyzer.SeverityCaution:
		return SeverityCaution
	}
	return SeverityInfo
}

func severityFromPlan(s planregression.Severity) Severity {
	switch s {
	case planregression.SeverityStop:
		return SeverityStop
	case planregression.SeverityCaution:
		return SeverityCaution
	}
	return SeverityInfo
}

func migrationDurationOf(r *executor.Result) time.Duration {
	if r == nil {
		return 0
	}
	return r.TotalDuration
}

// sortFindings orders findings severity-desc, then by the canonical
// group order, then alphabetically by object. The sort is stable so
// findings with identical keys preserve the order the builder
// produced them (matching the analyzer's own ordering).
func sortFindings(findings []Finding) {
	groupRank := make(map[Group]int, len(CanonicalGroupOrder))
	for i, g := range CanonicalGroupOrder {
		groupRank[g] = i
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity.Rank() > findings[j].Severity.Rank()
		}
		if findings[i].Group != findings[j].Group {
			return groupRank[findings[i].Group] < groupRank[findings[j].Group]
		}
		return findings[i].Object < findings[j].Object
	})
}
