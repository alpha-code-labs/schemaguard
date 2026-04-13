package planregression

// Severity mirrors lockanalyzer.Severity in spirit — info/caution/stop
// — but lives here so the two analyzers stay decoupled. The M5 report
// generator will unify them into a single verdict when it lands; M4
// intentionally does not aggregate across analyzers.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityCaution
	SeverityStop
)

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

// FindingKind is the reason the analyzer flagged a query. Each kind
// corresponds to exactly one of the bullet points in tasks.md 4.5.
type FindingKind int

const (
	// KindCostIncrease — the top-level estimated cost rose past the
	// caution or stop threshold.
	KindCostIncrease FindingKind = iota

	// KindRowsIncrease — the top-level estimated rows rose past the
	// caution or stop threshold.
	KindRowsIncrease

	// KindScanDowngrade — a scan node on a user relation moved to a
	// less efficient scan mode (e.g. Index Scan → Bitmap Heap Scan)
	// without becoming a full Seq Scan.
	KindScanDowngrade

	// KindNewSeqScan — a scan on a user relation that used an index
	// in the baseline is a Seq Scan post-migration. This is the
	// canonical "lost the index" regression.
	KindNewSeqScan

	// KindQueryBroken — the query could not be EXPLAINed against
	// the post-migration schema. Usually a dropped/renamed column or
	// table referenced in the query. This is caused BY the migration
	// and is always stop-severity.
	KindQueryBroken

	// KindBaselineBroken — the query could not be EXPLAINed against
	// the baseline schema even before the migration ran. This is a
	// user-config issue (the query in schemaguard.yaml is stale or
	// typo'd), not a migration regression. It is classified
	// separately from KindQueryBroken so reviewers do not mistake a
	// pre-existing config issue for a regression the migration
	// introduced. Always info-severity.
	KindBaselineBroken
)

// Finding is the structured output of the plan-regression analyzer
// for one query. A query can produce multiple findings on the same
// run (e.g. cost increase AND scan downgrade).
type Finding struct {
	QueryID  string
	Kind     FindingKind
	Severity Severity
	Reason   string

	// Top-level plan metrics, populated for cost/rows findings. For
	// KindQueryBroken and KindScanDowngrade / KindNewSeqScan these
	// may be zero.
	BaselineCost float64
	PostCost     float64
	BaselineRows float64
	PostRows     float64

	// Relation and scan-mode fields, populated for scan-change
	// findings. Empty otherwise.
	Relation     string
	BaselineScan string
	PostScan     string

	// ErrorMessage carries the Postgres error returned by the post-
	// migration EXPLAIN for KindQueryBroken findings. Empty
	// otherwise.
	ErrorMessage string
}

// Thresholds are the committed caution/stop thresholds the analyzer
// applies to plan-level cost and rows metrics. An instance is
// constructed by DefaultThresholds() and mutated by the config
// loader when user overrides are provided (see Config.
// EffectiveThresholds).
type Thresholds struct {
	CautionCostRatio float64
	StopCostRatio    float64
	MinCostDelta     float64

	CautionRowsRatio float64
	StopRowsRatio    float64
	MinRowsDelta     float64
}

// DefaultThresholds returns the first-pass M4 threshold defaults.
// These are committed in docs/DECISIONS.md and will be tuned against
// design-partner data. See tasks.md 4.6 for the ownership entry.
//
// Rationale (short form — full rationale lives in DECISIONS.md):
//
//   - Cost ratio 2.0 / 5.0: Postgres planner costs are abstract
//     units, but a doubling is a meaningful enough signal to warn
//     a reviewer. A 5x jump is rarely benign.
//   - Min cost delta 100: suppresses false positives on tiny
//     queries where the ratio would hit on absolute costs near 0.
//   - Rows ratio 10 / 100: an estimated-rows jump by an order of
//     magnitude is worth a caution; two orders of magnitude is a
//     stop.
//   - Min rows delta 1000: same rationale as min cost delta.
func DefaultThresholds() Thresholds {
	return Thresholds{
		CautionCostRatio: 2.0,
		StopCostRatio:    5.0,
		MinCostDelta:     100.0,
		CautionRowsRatio: 10.0,
		StopRowsRatio:    100.0,
		MinRowsDelta:     1000.0,
	}
}
