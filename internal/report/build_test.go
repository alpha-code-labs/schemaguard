package report

import (
	"errors"
	"testing"
	"time"

	"github.com/schemaguard/schemaguard/internal/executor"
	"github.com/schemaguard/schemaguard/internal/lockanalyzer"
	"github.com/schemaguard/schemaguard/internal/planregression"
)

func TestSchemaVersionIsPinned(t *testing.T) {
	if SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want %q (change requires DECISIONS.md update)", SchemaVersion, "1")
	}
}

func TestVerdictRankOrder(t *testing.T) {
	if VerdictRed.Rank() <= VerdictYellow.Rank() {
		t.Error("red rank should exceed yellow")
	}
	if VerdictYellow.Rank() <= VerdictGreen.Rank() {
		t.Error("yellow rank should exceed green")
	}
}

func TestVerdictExitCodes(t *testing.T) {
	cases := map[Verdict]int{
		VerdictGreen:  0,
		VerdictYellow: 1,
		VerdictRed:    2,
	}
	for v, want := range cases {
		if got := v.ExitCode(); got != want {
			t.Errorf("%s.ExitCode() = %d, want %d", v, got, want)
		}
	}
}

func TestSeverityRankOrder(t *testing.T) {
	if SeverityStop.Rank() <= SeverityCaution.Rank() {
		t.Error("stop rank should exceed caution")
	}
	if SeverityCaution.Rank() <= SeverityInfo.Rank() {
		t.Error("caution rank should exceed info")
	}
}

func TestBuildCleanRunIsGreen(t *testing.T) {
	r := Build(Input{
		ToolVersion:     "0.0.0-dev",
		MigrationResult: &executor.Result{},
	})
	if r.Verdict != VerdictGreen {
		t.Errorf("verdict = %s, want green", r.Verdict)
	}
	if len(r.Findings) != 0 {
		t.Errorf("expected no findings, got %d", len(r.Findings))
	}
	if r.Summary == "" {
		t.Error("summary should be non-empty")
	}
	if r.Verdict.ExitCode() != 0 {
		t.Errorf("exit code = %d, want 0", r.Verdict.ExitCode())
	}
}

func TestBuildOnlyCautionIsYellow(t *testing.T) {
	r := Build(Input{
		MigrationResult: &executor.Result{},
		LockFindings: []lockanalyzer.Finding{
			{
				Kind:     lockanalyzer.KindLock,
				Mode:     "ExclusiveLock",
				Object:   "public.orders",
				Duration: 500 * time.Millisecond,
				Severity: lockanalyzer.SeverityCaution,
				Reason:   "exclusive lock for 500ms",
			},
		},
	})
	if r.Verdict != VerdictYellow {
		t.Errorf("verdict = %s, want yellow", r.Verdict)
	}
	if r.Verdict.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", r.Verdict.ExitCode())
	}
}

func TestBuildOnlyStopIsRed(t *testing.T) {
	r := Build(Input{
		MigrationResult: &executor.Result{},
		PlanFindings: []planregression.Finding{
			{
				QueryID:  "q1",
				Kind:     planregression.KindNewSeqScan,
				Severity: planregression.SeverityStop,
				Reason:   "regressed to seq scan",
			},
		},
	})
	if r.Verdict != VerdictRed {
		t.Errorf("verdict = %s, want red", r.Verdict)
	}
	if r.Verdict.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", r.Verdict.ExitCode())
	}
}

func TestBuildFailedMigrationIsRedAndAddsMigrationExecutionFinding(t *testing.T) {
	failErr := errors.New("ERROR: column already exists")
	r := Build(Input{
		MigrationResult: &executor.Result{
			Failed:       true,
			FailureIndex: 1,
			FailureErr:   failErr,
		},
		LockFindings: []lockanalyzer.Finding{
			{
				Kind:     lockanalyzer.KindLock,
				Mode:     "AccessExclusiveLock",
				Object:   "public.orders",
				Duration: 2 * time.Second,
				Severity: lockanalyzer.SeverityStop,
				Reason:   "long lock on orders",
			},
		},
		PlanFindings: []planregression.Finding{
			// Should be dropped by Build on failure.
			{
				QueryID:  "q1",
				Kind:     planregression.KindCostIncrease,
				Severity: planregression.SeverityCaution,
				Reason:   "cost rose",
			},
		},
	})

	if r.Verdict != VerdictRed {
		t.Errorf("verdict = %s, want red", r.Verdict)
	}
	if r.Verdict.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", r.Verdict.ExitCode())
	}

	// Must contain exactly one MigrationExecution finding at stop
	// severity, referencing statement #2 (1-based).
	var migs []Finding
	for _, f := range r.Findings {
		if f.Group == GroupMigrationExecution {
			migs = append(migs, f)
		}
	}
	if len(migs) != 1 {
		t.Fatalf("expected 1 MigrationExecution finding, got %d", len(migs))
	}
	if migs[0].Severity != SeverityStop {
		t.Errorf("migration exec severity = %s, want stop", migs[0].Severity)
	}
	if migs[0].Object != "statement #2" {
		t.Errorf("migration exec object = %q, want 'statement #2'", migs[0].Object)
	}

	// Plan findings must NOT appear in a failed-migration report.
	for _, f := range r.Findings {
		if f.Group == GroupQueryPlan {
			t.Errorf("unexpected plan finding on failed migration: %+v", f)
		}
	}

	// Lock findings from pre-failure statements MUST still appear.
	var gotLock bool
	for _, f := range r.Findings {
		if f.Group == GroupLockRisk {
			gotLock = true
		}
	}
	if !gotLock {
		t.Error("expected pre-failure lock finding to be preserved")
	}

	// Summary must explicitly say the migration halted.
	if r.Summary == "" || !contains(r.Summary, "halted") {
		t.Errorf("summary should mention halted migration, got %q", r.Summary)
	}
}

func TestBuildSortsSeverityDescending(t *testing.T) {
	r := Build(Input{
		MigrationResult: &executor.Result{},
		LockFindings: []lockanalyzer.Finding{
			{Mode: "ShareLock", Object: "a", Severity: lockanalyzer.SeverityInfo, Reason: "r"},
			{Mode: "AccessExclusiveLock", Object: "b", Severity: lockanalyzer.SeverityStop, Reason: "r"},
			{Mode: "ExclusiveLock", Object: "c", Severity: lockanalyzer.SeverityCaution, Reason: "r"},
		},
	})
	if len(r.Findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(r.Findings))
	}
	if r.Findings[0].Severity != SeverityStop {
		t.Errorf("first = %s, want stop", r.Findings[0].Severity)
	}
	if r.Findings[2].Severity != SeverityInfo {
		t.Errorf("last = %s, want info", r.Findings[2].Severity)
	}
}

func TestBuildFindingsByGroupReturnsOnlyMatching(t *testing.T) {
	r := Report{
		Findings: []Finding{
			{Group: GroupLockRisk, Severity: SeverityStop, Object: "a"},
			{Group: GroupQueryPlan, Severity: SeverityCaution, Object: "b"},
			{Group: GroupLockRisk, Severity: SeverityInfo, Object: "c"},
		},
	}
	got := r.FindingsByGroup(GroupLockRisk)
	if len(got) != 2 {
		t.Errorf("expected 2 lock findings, got %d", len(got))
	}
	for _, f := range got {
		if f.Group != GroupLockRisk {
			t.Errorf("non-lock finding leaked: %+v", f)
		}
	}
}

func TestBuildNilMigrationResultIsGreen(t *testing.T) {
	r := Build(Input{ToolVersion: "dev"})
	if r.Verdict != VerdictGreen {
		t.Errorf("verdict = %s, want green for nil migration result", r.Verdict)
	}
}

func TestCanonicalGroupOrderPutsMigrationExecutionFirst(t *testing.T) {
	if CanonicalGroupOrder[0] != GroupMigrationExecution {
		t.Errorf("first group = %s, want migration_execution", CanonicalGroupOrder[0])
	}
}

func contains(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
