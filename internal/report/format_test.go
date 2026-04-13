package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleReport() *Report {
	return &Report{
		SchemaVersion: "1",
		Verdict:       VerdictRed,
		Summary:       "Migration ran but raised 2 stop-level findings; do not merge.",
		Findings: []Finding{
			{
				Group:    GroupLockRisk,
				Severity: SeverityStop,
				Kind:     "table_rewrite",
				Object:   "AccessExclusiveLock on public.orders",
				Impact:   "held for 1.5s — blocks reads + writes",
				Reason:   "probable full table rewrite",
			},
			{
				Group:    GroupQueryPlan,
				Severity: SeverityStop,
				Kind:     "new_seq_scan",
				Object:   "orders_by_customer",
				Impact:   "Bitmap Heap Scan → Seq Scan on public.orders",
				Reason:   "scan on public.orders regressed to Seq Scan",
			},
		},
		Footer: Footer{
			ToolVersion:       "0.0.0-dev",
			RunDuration:       time.Second + 500*time.Millisecond,
			MigrationDuration: 500 * time.Millisecond,
			RestoreDuration:   300 * time.Millisecond,
			ShadowDBImage:     "postgres:16-alpine",
			ShadowDBSizeBytes: 12 * 1024 * 1024,
			DocsURL:           "https://github.com/schemaguard/schemaguard",
		},
	}
}

func TestFormatTextContainsVerdictSummaryAndGroups(t *testing.T) {
	out := FormatText(sampleReport())
	if !strings.Contains(out, "STOP") {
		t.Errorf("text output missing STOP label: %q", out)
	}
	if !strings.Contains(out, "Migration ran but raised") {
		t.Errorf("text output missing summary: %q", out)
	}
	if !strings.Contains(out, "Lock Risk") {
		t.Errorf("text output missing Lock Risk section: %q", out)
	}
	if !strings.Contains(out, "Query Plan Regressions") {
		t.Errorf("text output missing Query Plan Regressions section: %q", out)
	}
	if !strings.Contains(out, "SchemaGuard 0.0.0-dev") {
		t.Errorf("text output missing footer version: %q", out)
	}
}

func TestFormatTextOmitsEmptyGroups(t *testing.T) {
	r := &Report{
		Verdict: VerdictGreen,
		Summary: "clean",
		Footer:  Footer{ToolVersion: "dev"},
	}
	out := FormatText(r)
	if strings.Contains(out, "Lock Risk") {
		t.Errorf("empty report should not render Lock Risk section: %q", out)
	}
	if !strings.Contains(out, "No findings") {
		t.Errorf("empty report should say 'No findings': %q", out)
	}
}

func TestFormatJSONHasSchemaVersionAndStableKeys(t *testing.T) {
	raw, err := FormatJSON(sampleReport())
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(raw))
	}
	if decoded["schemaVersion"] != "1" {
		t.Errorf("schemaVersion = %v, want \"1\"", decoded["schemaVersion"])
	}
	if decoded["verdict"] != "red" {
		t.Errorf("verdict = %v, want red", decoded["verdict"])
	}
	if _, ok := decoded["summary"]; !ok {
		t.Error("missing summary key")
	}
	if _, ok := decoded["findings"]; !ok {
		t.Error("missing findings key")
	}
	footer, ok := decoded["footer"].(map[string]interface{})
	if !ok {
		t.Fatalf("footer is not an object")
	}
	if footer["toolVersion"] != "0.0.0-dev" {
		t.Errorf("toolVersion = %v", footer["toolVersion"])
	}
	if footer["runDurationMs"] == nil {
		t.Error("missing runDurationMs")
	}
}

func TestFormatJSONEmptyFindingsIsEmptyArrayNotNull(t *testing.T) {
	r := &Report{
		SchemaVersion: "1",
		Verdict:       VerdictGreen,
		Summary:       "clean",
	}
	raw, err := FormatJSON(r)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	if !strings.Contains(string(raw), `"findings": []`) {
		t.Errorf("expected findings: []\n%s", string(raw))
	}
}

func TestFormatMarkdownContainsHeadingAndTable(t *testing.T) {
	out := FormatMarkdown(sampleReport())
	if !strings.Contains(out, "## 🔴 Stop") {
		t.Errorf("markdown missing heading: %q", out)
	}
	if !strings.Contains(out, "### Lock Risk") {
		t.Errorf("markdown missing Lock Risk heading: %q", out)
	}
	if !strings.Contains(out, "| Severity | Object | Impact | Reason |") {
		t.Errorf("markdown missing table header: %q", out)
	}
	if !strings.Contains(out, "SchemaGuard `0.0.0-dev`") {
		t.Errorf("markdown missing footer: %q", out)
	}
	if strings.Contains(out, TruncationFooter[:20]) {
		t.Errorf("non-truncated report should not include the truncation footer: %q", out)
	}
}

func TestFormatMarkdownTruncatesUnderSmallBudget(t *testing.T) {
	r := sampleReport()
	// Add enough extra findings to overflow a tiny budget.
	for i := 0; i < 20; i++ {
		r.Findings = append(r.Findings, Finding{
			Group:    GroupLockRisk,
			Severity: SeverityInfo,
			Object:   "filler",
			Impact:   "info lock",
			Reason:   "noise",
		})
	}
	const smallBudget = 1200
	out := formatMarkdownWithBudget(r, smallBudget)
	if len(out) > smallBudget {
		t.Errorf("truncated output exceeded budget %d: len=%d", smallBudget, len(out))
	}
	if !strings.Contains(out, "Report truncated") {
		t.Errorf("expected truncation footer in output: %q", out)
	}
	// Must still contain the verdict line even after truncation.
	if !strings.Contains(out, "## 🔴") {
		t.Errorf("truncated output lost the verdict heading: %q", out)
	}
}

func TestFormatMarkdownDropsLowestSeverityFirst(t *testing.T) {
	r := &Report{
		SchemaVersion: "1",
		Verdict:       VerdictRed,
		Summary:       "s",
		Footer:        Footer{ToolVersion: "dev"},
	}
	// Sorted findings: stop, caution, info × N
	r.Findings = []Finding{
		{Group: GroupLockRisk, Severity: SeverityStop, Object: "KEEP_STOP", Impact: "x", Reason: "r"},
		{Group: GroupLockRisk, Severity: SeverityCaution, Object: "KEEP_CAUTION", Impact: "x", Reason: "r"},
	}
	for i := 0; i < 40; i++ {
		r.Findings = append(r.Findings, Finding{
			Group:    GroupLockRisk,
			Severity: SeverityInfo,
			Object:   "info-filler",
			Impact:   "x",
			Reason:   "r",
		})
	}
	// Tiny budget that forces aggressive drops.
	out := formatMarkdownWithBudget(r, 900)
	if !strings.Contains(out, "KEEP_STOP") {
		t.Errorf("truncation dropped the stop finding: %q", out)
	}
	// A tiny budget may force dropping even the caution row — but
	// the stop row must always survive until the bitter end.
}

func TestDefaultMarkdownBudgetMatchesDecision(t *testing.T) {
	if DefaultMarkdownBudget != 55000 {
		t.Errorf("DefaultMarkdownBudget = %d, want 55000 (see docs/DECISIONS.md)", DefaultMarkdownBudget)
	}
}

func TestHumanBytesBasic(t *testing.T) {
	cases := map[int64]string{
		0:         "0 B",
		1023:      "1023 B",
		1024:      "1.0 KB",
		1536:      "1.5 KB",
		1_048_576: "1.0 MB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
