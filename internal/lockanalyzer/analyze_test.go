package lockanalyzer

import (
	"testing"
	"time"
)

// baseTime is a fixed reference time used so test-window arithmetic
// is trivially readable. All relative offsets are in milliseconds.
var baseTime = time.Date(2026, 4, 11, 14, 0, 0, 0, time.UTC)

func at(ms int) time.Time {
	return baseTime.Add(time.Duration(ms) * time.Millisecond)
}

func window(idx, startMS, endMS int, sql string) StatementWindow {
	return StatementWindow{
		Index:     idx,
		StartedAt: at(startMS),
		EndAt:     at(endMS),
		SQL:       sql,
	}
}

func sample(ms int, mode, obj string, granted bool) Sample {
	return Sample{
		At:      at(ms),
		Mode:    mode,
		Granted: granted,
		Object:  obj,
	}
}

func TestAnalyzeEmptySamplesReturnsNil(t *testing.T) {
	windows := []StatementWindow{window(0, 0, 100, "SELECT 1")}
	got := Analyze(nil, windows, 50*time.Millisecond)
	if got != nil {
		t.Errorf("expected nil findings for empty samples, got %v", got)
	}
}

func TestAnalyzeGroupsByObjectAndMode(t *testing.T) {
	windows := []StatementWindow{window(0, 0, 200, "ALTER TABLE users ...")}
	samples := []Sample{
		sample(10, "AccessShareLock", "public.users", true),
		sample(60, "AccessShareLock", "public.users", true),
		sample(110, "AccessShareLock", "public.users", true),
		sample(160, "AccessShareLock", "public.users", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 grouped finding, got %d: %+v", len(got), got)
	}
	if got[0].Object != "public.users" || got[0].Mode != "AccessShareLock" {
		t.Errorf("unexpected finding: %+v", got[0])
	}
	// Duration is last - first + interval = 150 + 50 = 200ms.
	want := 200 * time.Millisecond
	if got[0].Duration != want {
		t.Errorf("duration = %s, want %s", got[0].Duration, want)
	}
}

func TestAnalyzeDistinctModesProduceDistinctFindings(t *testing.T) {
	windows := []StatementWindow{window(0, 0, 500, "ALTER TABLE users ...")}
	samples := []Sample{
		sample(10, "AccessShareLock", "public.users", true),
		sample(10, "AccessExclusiveLock", "public.users", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) != 2 {
		t.Fatalf("expected 2 findings (one per mode), got %d", len(got))
	}
}

func TestAnalyzeAttributesToContainingWindow(t *testing.T) {
	windows := []StatementWindow{
		window(0, 0, 100, "SELECT 1"),
		window(1, 100, 300, "ALTER TABLE users ..."),
		window(2, 300, 400, "SELECT 2"),
	}
	samples := []Sample{
		sample(150, "AccessExclusiveLock", "public.users", true),
		sample(200, "AccessExclusiveLock", "public.users", true),
		sample(250, "AccessExclusiveLock", "public.users", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].StatementIndex != 1 {
		t.Errorf("statement index = %d, want 1", got[0].StatementIndex)
	}
}

func TestAnalyzeAttributesToFirstSampleWindow(t *testing.T) {
	windows := []StatementWindow{
		window(0, 0, 100, "ALTER TABLE a ..."),
		window(1, 100, 200, "ALTER TABLE b ..."),
	}
	// First sample in window 0, later samples in window 1.
	// Attribution pins to the window of the FIRST sample.
	samples := []Sample{
		sample(50, "AccessExclusiveLock", "public.users", true),
		sample(150, "AccessExclusiveLock", "public.users", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].StatementIndex != 0 {
		t.Errorf("statement index = %d, want 0 (first-sample window)", got[0].StatementIndex)
	}
}

func TestAnalyzeSampleOutsideAnyWindowAttributesToNegativeOne(t *testing.T) {
	windows := []StatementWindow{window(0, 100, 200, "SELECT 1")}
	samples := []Sample{
		sample(50, "AccessShareLock", "public.users", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].StatementIndex != -1 {
		t.Errorf("statement index = %d, want -1 (no containing window)", got[0].StatementIndex)
	}
}

func TestAnalyzeNonGrantedPropagates(t *testing.T) {
	windows := []StatementWindow{window(0, 0, 200, "ALTER TABLE ...")}
	samples := []Sample{
		sample(50, "AccessExclusiveLock", "public.users", false),
		sample(100, "AccessExclusiveLock", "public.users", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].Granted {
		t.Errorf("Granted = true, want false (one sample was not granted)")
	}
}

func TestClassifySeverityAccessExclusive(t *testing.T) {
	cases := []struct {
		name string
		dur  time.Duration
		want Severity
	}{
		{"brief", 50 * time.Millisecond, SeverityInfo},
		{"caution window", 200 * time.Millisecond, SeverityCaution},
		{"stop window", 600 * time.Millisecond, SeverityStop},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifySeverity("AccessExclusiveLock", tc.dur); got != tc.want {
				t.Errorf("classifySeverity(AccessExclusiveLock, %s) = %v, want %v", tc.dur, got, tc.want)
			}
		})
	}
}

func TestClassifySeverityExclusive(t *testing.T) {
	if classifySeverity("ExclusiveLock", 100*time.Millisecond) != SeverityInfo {
		t.Error("ExclusiveLock 100ms should be info")
	}
	if classifySeverity("ShareRowExclusiveLock", 500*time.Millisecond) != SeverityCaution {
		t.Error("ShareRowExclusiveLock 500ms should be caution")
	}
	if classifySeverity("ExclusiveLock", 2*time.Second) != SeverityStop {
		t.Error("ExclusiveLock 2s should be stop")
	}
}

func TestClassifySeverityShare(t *testing.T) {
	if classifySeverity("ShareLock", 500*time.Millisecond) != SeverityInfo {
		t.Error("ShareLock 500ms should be info")
	}
	if classifySeverity("ShareUpdateExclusiveLock", 2*time.Second) != SeverityCaution {
		t.Error("ShareUpdateExclusiveLock 2s should be caution")
	}
	if classifySeverity("ShareLock", 10*time.Second) != SeverityStop {
		t.Error("ShareLock 10s should be stop")
	}
}

func TestClassifySeverityWeakLocksAlwaysInfo(t *testing.T) {
	for _, mode := range []string{"AccessShareLock", "RowShareLock", "RowExclusiveLock"} {
		if classifySeverity(mode, 10*time.Second) != SeverityInfo {
			t.Errorf("%s should always be info regardless of duration", mode)
		}
	}
}

func TestModeBlockingMatrix(t *testing.T) {
	if !modeBlocksReads("AccessExclusiveLock") {
		t.Error("AccessExclusiveLock must block reads")
	}
	if modeBlocksReads("ExclusiveLock") {
		t.Error("ExclusiveLock does not block reads in Postgres")
	}
	if !modeBlocksWrites("AccessExclusiveLock") {
		t.Error("AccessExclusiveLock must block writes")
	}
	if !modeBlocksWrites("ExclusiveLock") {
		t.Error("ExclusiveLock must block writes")
	}
	if modeBlocksWrites("AccessShareLock") {
		t.Error("AccessShareLock must not block writes")
	}
}

func TestAnalyzeFlagsTableRewrite(t *testing.T) {
	// A statement that took 500ms with an AccessExclusive lock held
	// throughout. This should be flagged as a table rewrite.
	windows := []StatementWindow{window(0, 0, 500, "ALTER TABLE users ADD COLUMN x ...")}
	samples := []Sample{
		sample(10, "AccessExclusiveLock", "public.users", true),
		sample(100, "AccessExclusiveLock", "public.users", true),
		sample(200, "AccessExclusiveLock", "public.users", true),
		sample(300, "AccessExclusiveLock", "public.users", true),
		sample(400, "AccessExclusiveLock", "public.users", true),
		sample(450, "AccessExclusiveLock", "public.users", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].Kind != KindTableRewrite {
		t.Errorf("Kind = %v, want KindTableRewrite", got[0].Kind)
	}
	if got[0].Severity < SeverityCaution {
		t.Errorf("Severity = %v, want at least SeverityCaution", got[0].Severity)
	}
}

func TestAnalyzeDoesNotFlagBriefExclusiveAsRewrite(t *testing.T) {
	// Short ACCESS EXCLUSIVE on a fast statement — metadata-only ALTER.
	// Should not be flagged as a table rewrite.
	windows := []StatementWindow{window(0, 0, 30, "ALTER TABLE users ADD COLUMN x TEXT")}
	samples := []Sample{
		sample(5, "AccessExclusiveLock", "public.users", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].Kind != KindLock {
		t.Errorf("Kind = %v, want KindLock (not rewrite — statement too short)", got[0].Kind)
	}
}

func TestAnalyzeDoesNotFlagNonExclusiveAsRewrite(t *testing.T) {
	// Long SHARE lock does not count as a rewrite even if held long.
	windows := []StatementWindow{window(0, 0, 2000, "ALTER TABLE users ...")}
	samples := []Sample{
		sample(10, "ShareLock", "public.users", true),
		sample(500, "ShareLock", "public.users", true),
		sample(1000, "ShareLock", "public.users", true),
		sample(1500, "ShareLock", "public.users", true),
		sample(1900, "ShareLock", "public.users", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].Kind != KindLock {
		t.Errorf("Kind = %v, want KindLock (non-AccessExclusive is never a rewrite)", got[0].Kind)
	}
}

func TestAnalyzeSortsBySeverityDescending(t *testing.T) {
	windows := []StatementWindow{
		window(0, 0, 500, "ALTER TABLE a ..."),
		window(1, 500, 1000, "ALTER TABLE b ..."),
	}
	samples := []Sample{
		// Statement 0: long AccessExclusive (stop severity).
		sample(10, "AccessExclusiveLock", "public.a", true),
		sample(400, "AccessExclusiveLock", "public.a", true),
		// Statement 1: brief AccessShare (info).
		sample(600, "AccessShareLock", "public.b", true),
	}
	got := Analyze(samples, windows, 50*time.Millisecond)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 findings, got %d", len(got))
	}
	if got[0].Severity < got[1].Severity {
		t.Errorf("findings not sorted by severity descending: %v → %v", got[0].Severity, got[1].Severity)
	}
}

func TestDefaultSamplingIntervalMatchesDecision(t *testing.T) {
	// This guard keeps the sampler constant and the DECISIONS.md entry
	// in lockstep. If the value changes, update DECISIONS.md too.
	if DefaultSamplingInterval != 50*time.Millisecond {
		t.Errorf("DefaultSamplingInterval = %s, want 50ms (see docs/DECISIONS.md)", DefaultSamplingInterval)
	}
}
