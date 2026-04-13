package planregression

import (
	"fmt"
	"sort"
)

// Analyze diffs baseline vs post capture results and produces plan-
// regression findings. It is deterministic, reads only the raw plan
// JSON (and any capture error), and never runs queries.
//
// The caller is responsible for NOT calling Analyze on a failed
// migration run: per docs/DECISIONS.md and tasks.md 4.4, post-
// migration plan capture is skipped entirely when the migration
// halted on error, and the analyzer produces no findings for that
// run.
//
// Empty inputs yield empty output. Queries present in baseline but
// missing from post (or vice versa) are ignored — M4 does not model
// partial capture.
func Analyze(baseline, post []CaptureResult, t Thresholds) []Finding {
	if len(baseline) == 0 || len(post) == 0 {
		return nil
	}
	postByID := make(map[string]CaptureResult, len(post))
	for _, p := range post {
		postByID[p.QueryID] = p
	}

	var findings []Finding
	for _, b := range baseline {
		p, ok := postByID[b.QueryID]
		if !ok {
			continue
		}
		findings = append(findings, compareOne(b, p, t)...)
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity > findings[j].Severity
		}
		if findings[i].QueryID != findings[j].QueryID {
			return findings[i].QueryID < findings[j].QueryID
		}
		return findings[i].Kind < findings[j].Kind
	})
	return findings
}

// compareOne produces zero or more findings for a single
// baseline/post pair. The order of checks matters only for output
// stability — callers should not assume any ordering beyond the
// final sort in Analyze.
func compareOne(b, p CaptureResult, t Thresholds) []Finding {
	var out []Finding

	// Broken-query check first: a query that cannot EXPLAIN against
	// the post-migration schema is always red and short-circuits
	// any further plan comparison for that query.
	if p.HasError() {
		return []Finding{{
			QueryID:      p.QueryID,
			Kind:         KindQueryBroken,
			Severity:     SeverityStop,
			Reason:       fmt.Sprintf("post-migration EXPLAIN failed: %v", p.Err),
			ErrorMessage: fmt.Sprintf("%v", p.Err),
		}}
	}

	// If the baseline itself could not EXPLAIN, we have nothing to
	// diff against. This is a CONFIG issue (a query in the user's
	// schemaguard.yaml is stale or typo'd), NOT a regression caused
	// by the migration. Classify it with a distinct kind so
	// reviewers do not mistake it for a migration-introduced break.
	// Severity stays info — the run is not regressing anything.
	if b.HasError() {
		return []Finding{{
			QueryID:      b.QueryID,
			Kind:         KindBaselineBroken,
			Severity:     SeverityInfo,
			Reason:       fmt.Sprintf("config issue: baseline EXPLAIN of query %q failed before the migration ran — fix or remove it in schemaguard.yaml: %v", b.QueryID, b.Err),
			ErrorMessage: fmt.Sprintf("%v", b.Err),
		}}
	}

	bCost, pCost := b.Plan.totalCost(), p.Plan.totalCost()
	bRows, pRows := b.Plan.planRows(), p.Plan.planRows()

	// Cost increase.
	if f, ok := checkCost(b.QueryID, bCost, pCost, t); ok {
		out = append(out, f)
	}
	// Rows increase.
	if f, ok := checkRows(b.QueryID, bRows, pRows, t); ok {
		out = append(out, f)
	}
	// Scan changes (Seq Scan introductions, scan downgrades).
	out = append(out, checkScans(b.QueryID, b.Plan, p.Plan, bCost, pCost, bRows, pRows)...)

	return out
}

// checkCost flags cost-increase regressions. A finding is produced
// when the post/baseline ratio exceeds the caution or stop ratio
// AND the absolute delta exceeds MinCostDelta. Both conditions must
// hold so tiny queries with near-zero costs do not fire on noise.
func checkCost(id string, bCost, pCost float64, t Thresholds) (Finding, bool) {
	if bCost <= 0 || pCost <= bCost {
		return Finding{}, false
	}
	delta := pCost - bCost
	if delta < t.MinCostDelta {
		return Finding{}, false
	}
	ratio := pCost / bCost
	switch {
	case ratio >= t.StopCostRatio:
		return Finding{
			QueryID:      id,
			Kind:         KindCostIncrease,
			Severity:     SeverityStop,
			Reason:       fmt.Sprintf("estimated cost rose %.1fx (%.0f → %.0f)", ratio, bCost, pCost),
			BaselineCost: bCost,
			PostCost:     pCost,
		}, true
	case ratio >= t.CautionCostRatio:
		return Finding{
			QueryID:      id,
			Kind:         KindCostIncrease,
			Severity:     SeverityCaution,
			Reason:       fmt.Sprintf("estimated cost rose %.1fx (%.0f → %.0f)", ratio, bCost, pCost),
			BaselineCost: bCost,
			PostCost:     pCost,
		}, true
	}
	return Finding{}, false
}

// checkRows flags estimated-rows-increase regressions. The rule is
// the same shape as checkCost but uses rows thresholds. Both the
// ratio and the absolute delta must exceed the relevant threshold.
func checkRows(id string, bRows, pRows float64, t Thresholds) (Finding, bool) {
	if bRows <= 0 || pRows <= bRows {
		return Finding{}, false
	}
	delta := pRows - bRows
	if delta < t.MinRowsDelta {
		return Finding{}, false
	}
	ratio := pRows / bRows
	switch {
	case ratio >= t.StopRowsRatio:
		return Finding{
			QueryID:      id,
			Kind:         KindRowsIncrease,
			Severity:     SeverityStop,
			Reason:       fmt.Sprintf("estimated rows rose %.1fx (%.0f → %.0f)", ratio, bRows, pRows),
			BaselineRows: bRows,
			PostRows:     pRows,
		}, true
	case ratio >= t.CautionRowsRatio:
		return Finding{
			QueryID:      id,
			Kind:         KindRowsIncrease,
			Severity:     SeverityCaution,
			Reason:       fmt.Sprintf("estimated rows rose %.1fx (%.0f → %.0f)", ratio, bRows, pRows),
			BaselineRows: bRows,
			PostRows:     pRows,
		}, true
	}
	return Finding{}, false
}

// checkScans walks the two plan trees and produces a finding per
// relation that moved to a worse scan mode. A new Seq Scan where an
// index scan used to exist is a dedicated finding kind and is always
// stop-severity. Lesser downgrades are caution.
func checkScans(id string, bPlan, pPlan planNode, bCost, pCost, bRows, pRows float64) []Finding {
	bScans := scansByRelation(bPlan)
	pScans := scansByRelation(pPlan)

	var out []Finding
	// Iterate in sorted relation order so output is deterministic
	// across runs — Go map iteration is randomized otherwise.
	relations := make([]string, 0, len(pScans))
	for rel := range pScans {
		relations = append(relations, rel)
	}
	sort.Strings(relations)

	for _, rel := range relations {
		postScan := pScans[rel]
		baselineScan, hadBaseline := bScans[rel]
		if !hadBaseline {
			// Relation appears only in the post plan — typical
			// when a JOIN shape changes. M4 does not model this;
			// ignore.
			continue
		}
		if scanRank(postScan) <= scanRank(baselineScan) {
			continue
		}
		if postScan == "Seq Scan" {
			out = append(out, Finding{
				QueryID:      id,
				Kind:         KindNewSeqScan,
				Severity:     SeverityStop,
				Reason:       fmt.Sprintf("scan on %s regressed from %s to Seq Scan", rel, baselineScan),
				BaselineCost: bCost,
				PostCost:     pCost,
				BaselineRows: bRows,
				PostRows:     pRows,
				Relation:     rel,
				BaselineScan: baselineScan,
				PostScan:     postScan,
			})
			continue
		}
		out = append(out, Finding{
			QueryID:      id,
			Kind:         KindScanDowngrade,
			Severity:     SeverityCaution,
			Reason:       fmt.Sprintf("scan on %s downgraded from %s to %s", rel, baselineScan, postScan),
			BaselineCost: bCost,
			PostCost:     pCost,
			BaselineRows: bRows,
			PostRows:     pRows,
			Relation:     rel,
			BaselineScan: baselineScan,
			PostScan:     postScan,
		})
	}
	return out
}
