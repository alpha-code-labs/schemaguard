package planregression

import (
	"errors"
	"strings"
	"testing"
)

// newScanNode builds a minimal plan node shaped like one Postgres
// EXPLAIN (FORMAT JSON) would emit for a single scan. It is used by
// unit tests — the real analyzer reads the same shape from pgx.
func newScanNode(nodeType, schema, rel string, cost, rows float64) planNode {
	return planNode{
		"Node Type":     nodeType,
		"Relation Name": rel,
		"Schema":        schema,
		"Total Cost":    cost,
		"Plan Rows":     rows,
	}
}

// withChildren returns parent with a "Plans" array wrapping children.
func withChildren(parent planNode, children ...planNode) planNode {
	arr := make([]interface{}, 0, len(children))
	for _, c := range children {
		arr = append(arr, map[string]interface{}(c))
	}
	parent["Plans"] = arr
	return parent
}

func TestAnalyzeEmptyInputsReturnNil(t *testing.T) {
	if got := Analyze(nil, nil, DefaultThresholds()); got != nil {
		t.Errorf("expected nil for nil inputs, got %v", got)
	}
	bs := []CaptureResult{{QueryID: "q1", Plan: newScanNode("Seq Scan", "public", "t", 100, 10)}}
	if got := Analyze(bs, nil, DefaultThresholds()); got != nil {
		t.Errorf("expected nil when post is empty, got %v", got)
	}
	if got := Analyze(nil, bs, DefaultThresholds()); got != nil {
		t.Errorf("expected nil when baseline is empty, got %v", got)
	}
}

func TestAnalyzeNoChangeProducesNothing(t *testing.T) {
	b := []CaptureResult{{QueryID: "q1", Plan: newScanNode("Index Scan", "public", "orders", 50, 10)}}
	p := []CaptureResult{{QueryID: "q1", Plan: newScanNode("Index Scan", "public", "orders", 50, 10)}}
	if got := Analyze(b, p, DefaultThresholds()); len(got) != 0 {
		t.Errorf("expected no findings, got %v", got)
	}
}

func TestAnalyzeQueryBrokenPostMigrationIsStop(t *testing.T) {
	b := []CaptureResult{{QueryID: "q1", Plan: newScanNode("Index Scan", "public", "orders", 50, 10)}}
	p := []CaptureResult{{QueryID: "q1", Err: errors.New("column \"customer_id\" does not exist")}}
	got := Analyze(b, p, DefaultThresholds())
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %v", len(got), got)
	}
	if got[0].Kind != KindQueryBroken {
		t.Errorf("Kind = %v, want KindQueryBroken", got[0].Kind)
	}
	if got[0].Severity != SeverityStop {
		t.Errorf("Severity = %v, want SeverityStop", got[0].Severity)
	}
	if got[0].ErrorMessage == "" {
		t.Error("ErrorMessage should be populated")
	}
}

func TestAnalyzeBaselineBrokenIsBaselineBrokenKindInfoAndNoDiff(t *testing.T) {
	b := []CaptureResult{{QueryID: "q1", Err: errors.New("syntax error")}}
	p := []CaptureResult{{QueryID: "q1", Plan: newScanNode("Seq Scan", "public", "orders", 1000, 100)}}
	got := Analyze(b, p, DefaultThresholds())
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].Kind != KindBaselineBroken {
		t.Errorf("Kind = %v, want KindBaselineBroken (distinct from KindQueryBroken)", got[0].Kind)
	}
	if got[0].Severity != SeverityInfo {
		t.Errorf("Severity = %v, want SeverityInfo", got[0].Severity)
	}
	// The reason string should make it clear this is a config issue,
	// not a migration-caused regression.
	if got[0].Reason == "" || !containsAny(got[0].Reason, "config issue", "schemaguard.yaml") {
		t.Errorf("Reason should flag config issue, got %q", got[0].Reason)
	}
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func TestAnalyzeCostCautionAtRatio2x(t *testing.T) {
	b := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 100.0, "Plan Rows": 10.0}}}
	p := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 300.0, "Plan Rows": 10.0}}}
	got := Analyze(b, p, DefaultThresholds())
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %v", len(got), got)
	}
	if got[0].Kind != KindCostIncrease || got[0].Severity != SeverityCaution {
		t.Errorf("finding = %+v, want cost-increase/caution", got[0])
	}
}

func TestAnalyzeCostStopAtRatio5x(t *testing.T) {
	b := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 100.0, "Plan Rows": 10.0}}}
	p := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 700.0, "Plan Rows": 10.0}}}
	got := Analyze(b, p, DefaultThresholds())
	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(got))
	}
	if got[0].Severity != SeverityStop {
		t.Errorf("Severity = %v, want SeverityStop", got[0].Severity)
	}
}

func TestAnalyzeCostBelowMinDeltaDoesNotFire(t *testing.T) {
	// Even a 10x ratio should not fire when the absolute delta is
	// smaller than MinCostDelta (default 100). 5 → 50 is 10x ratio
	// but only 45 delta.
	b := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 5.0, "Plan Rows": 1.0}}}
	p := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 50.0, "Plan Rows": 1.0}}}
	got := Analyze(b, p, DefaultThresholds())
	if len(got) != 0 {
		t.Errorf("expected no finding (delta below MinCostDelta), got %v", got)
	}
}

func TestAnalyzeRowsCautionAtRatio10x(t *testing.T) {
	b := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 1.0, "Plan Rows": 500.0}}}
	p := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 1.0, "Plan Rows": 6000.0}}}
	got := Analyze(b, p, DefaultThresholds())
	if len(got) == 0 {
		t.Fatal("expected rows-increase finding")
	}
	var found bool
	for _, f := range got {
		if f.Kind == KindRowsIncrease {
			found = true
			if f.Severity != SeverityCaution {
				t.Errorf("rows severity = %v, want caution", f.Severity)
			}
		}
	}
	if !found {
		t.Errorf("no KindRowsIncrease in %v", got)
	}
}

func TestAnalyzeRowsBelowMinDeltaDoesNotFire(t *testing.T) {
	b := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 1.0, "Plan Rows": 10.0}}}
	p := []CaptureResult{{QueryID: "q1", Plan: planNode{"Total Cost": 1.0, "Plan Rows": 200.0}}}
	got := Analyze(b, p, DefaultThresholds())
	for _, f := range got {
		if f.Kind == KindRowsIncrease {
			t.Errorf("unexpected rows finding (below MinRowsDelta): %+v", f)
		}
	}
}

func TestAnalyzeNewSeqScanIsStop(t *testing.T) {
	b := []CaptureResult{{
		QueryID: "q1",
		Plan:    newScanNode("Index Scan", "public", "orders", 50, 10),
	}}
	p := []CaptureResult{{
		QueryID: "q1",
		Plan:    newScanNode("Seq Scan", "public", "orders", 50, 10),
	}}
	got := Analyze(b, p, DefaultThresholds())
	if len(got) == 0 {
		t.Fatal("expected new-seq-scan finding")
	}
	var found bool
	for _, f := range got {
		if f.Kind == KindNewSeqScan {
			found = true
			if f.Severity != SeverityStop {
				t.Errorf("new seq scan severity = %v, want stop", f.Severity)
			}
			if f.Relation != "public.orders" {
				t.Errorf("relation = %q, want public.orders", f.Relation)
			}
			if f.BaselineScan != "Index Scan" {
				t.Errorf("baseline scan = %q", f.BaselineScan)
			}
			if f.PostScan != "Seq Scan" {
				t.Errorf("post scan = %q", f.PostScan)
			}
		}
	}
	if !found {
		t.Errorf("no KindNewSeqScan in %v", got)
	}
}

func TestAnalyzeScanDowngradeIsCaution(t *testing.T) {
	b := []CaptureResult{{
		QueryID: "q1",
		Plan:    newScanNode("Index Scan", "public", "orders", 50, 10),
	}}
	p := []CaptureResult{{
		QueryID: "q1",
		Plan:    newScanNode("Bitmap Heap Scan", "public", "orders", 50, 10),
	}}
	got := Analyze(b, p, DefaultThresholds())
	var found bool
	for _, f := range got {
		if f.Kind == KindScanDowngrade {
			found = true
			if f.Severity != SeverityCaution {
				t.Errorf("downgrade severity = %v, want caution", f.Severity)
			}
		}
	}
	if !found {
		t.Errorf("no KindScanDowngrade in %v (got %v)", got, got)
	}
}

func TestAnalyzeScanUpgradeNotFlagged(t *testing.T) {
	// A post plan that is STRICTLY better than baseline should not
	// produce any scan-change finding.
	b := []CaptureResult{{
		QueryID: "q1",
		Plan:    newScanNode("Seq Scan", "public", "orders", 1000, 1000),
	}}
	p := []CaptureResult{{
		QueryID: "q1",
		Plan:    newScanNode("Index Scan", "public", "orders", 50, 10),
	}}
	got := Analyze(b, p, DefaultThresholds())
	for _, f := range got {
		if f.Kind == KindScanDowngrade || f.Kind == KindNewSeqScan {
			t.Errorf("unexpected scan-regression finding for upgrade: %+v", f)
		}
	}
}

func TestAnalyzeSortsBySeverityDescending(t *testing.T) {
	b := []CaptureResult{
		{QueryID: "a_low", Plan: planNode{"Total Cost": 100.0, "Plan Rows": 10.0}},
		{QueryID: "b_high", Plan: planNode{"Total Cost": 100.0, "Plan Rows": 10.0}},
	}
	p := []CaptureResult{
		// a_low: 2.5x caution
		{QueryID: "a_low", Plan: planNode{"Total Cost": 250.0, "Plan Rows": 10.0}},
		// b_high: 6x stop
		{QueryID: "b_high", Plan: planNode{"Total Cost": 600.0, "Plan Rows": 10.0}},
	}
	got := Analyze(b, p, DefaultThresholds())
	if len(got) < 2 {
		t.Fatalf("expected >=2 findings, got %d", len(got))
	}
	if got[0].Severity < got[1].Severity {
		t.Errorf("findings not sorted by severity desc: %v → %v", got[0].Severity, got[1].Severity)
	}
}

func TestScansByRelationPicksWorstScanPerRelation(t *testing.T) {
	root := withChildren(
		planNode{"Node Type": "Nested Loop"},
		newScanNode("Index Scan", "public", "orders", 1, 1),
		newScanNode("Seq Scan", "public", "orders", 1, 1),
	)
	got := scansByRelation(root)
	if got["public.orders"] != "Seq Scan" {
		t.Errorf("scansByRelation picked %q, want Seq Scan (the worst)", got["public.orders"])
	}
}

func TestScansByRelationIgnoresNonScanNodes(t *testing.T) {
	root := planNode{
		"Node Type": "Aggregate",
	}
	got := scansByRelation(root)
	if len(got) != 0 {
		t.Errorf("expected no scans, got %v", got)
	}
}

func TestScanRankOrdering(t *testing.T) {
	if scanRank("Index Only Scan") >= scanRank("Index Scan") {
		t.Error("Index Only Scan must rank lower than Index Scan")
	}
	if scanRank("Index Scan") >= scanRank("Bitmap Heap Scan") {
		t.Error("Index Scan must rank lower than Bitmap Heap Scan")
	}
	if scanRank("Bitmap Heap Scan") >= scanRank("Seq Scan") {
		t.Error("Bitmap Heap Scan must rank lower than Seq Scan")
	}
}

func TestParseExplainJSONRoundtrip(t *testing.T) {
	raw := []byte(`[{"Plan":{"Node Type":"Seq Scan","Relation Name":"orders","Schema":"public","Total Cost":100.5,"Plan Rows":50}}]`)
	plan, err := parseExplainJSON(raw)
	if err != nil {
		t.Fatalf("parseExplainJSON: %v", err)
	}
	if plan.nodeType() != "Seq Scan" {
		t.Errorf("nodeType = %q", plan.nodeType())
	}
	if plan.relationName() != "public.orders" {
		t.Errorf("relationName = %q", plan.relationName())
	}
	if plan.totalCost() != 100.5 {
		t.Errorf("totalCost = %v", plan.totalCost())
	}
	if plan.planRows() != 50 {
		t.Errorf("planRows = %v", plan.planRows())
	}
}

func TestSeverityStringStable(t *testing.T) {
	cases := map[Severity]string{
		SeverityInfo:    "info",
		SeverityCaution: "caution",
		SeverityStop:    "stop",
	}
	for s, want := range cases {
		if s.String() != want {
			t.Errorf("Severity(%d).String() = %q, want %q", s, s.String(), want)
		}
	}
}
