package planregression

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// TopQuery is one user-specified query that the analyzer should
// EXPLAIN before and after the migration. ID is a short stable
// identifier the user picks for their own reference; it flows into
// every Finding produced for this query.
type TopQuery struct {
	ID  string `yaml:"id"`
	SQL string `yaml:"sql"`
}

// ThresholdOverrides holds optional per-project overrides for the
// M4 plan-regression thresholds. Every field is a pointer so the
// loader can distinguish "omitted" from "explicitly set to 0". A
// nil pointer means "use the committed default from finding.go".
//
// Per docs/DECISIONS.md the M4 config file exposes ONLY query-plan
// regression thresholds. Lock severity thresholds and the rewrite
// heuristic are committed constants in internal/lockanalyzer and
// are not user-configurable in v1.
type ThresholdOverrides struct {
	CautionCostRatio *float64 `yaml:"caution_cost_ratio,omitempty"`
	StopCostRatio    *float64 `yaml:"stop_cost_ratio,omitempty"`
	MinCostDelta     *float64 `yaml:"min_cost_delta,omitempty"`
	CautionRowsRatio *float64 `yaml:"caution_rows_ratio,omitempty"`
	StopRowsRatio    *float64 `yaml:"stop_rows_ratio,omitempty"`
	MinRowsDelta     *float64 `yaml:"min_rows_delta,omitempty"`
}

// Config is the full YAML schema for schemaguard's config file.
// Everything is optional: a config with zero queries and no
// threshold section is valid but produces no plan findings.
type Config struct {
	Queries    []TopQuery          `yaml:"queries"`
	Thresholds *ThresholdOverrides `yaml:"thresholds,omitempty"`
}

// EffectiveThresholds returns the threshold set that applies after
// merging the config's optional overrides on top of the committed
// first-pass defaults. It never returns nil and is safe to call on
// a Config with no Thresholds section.
func (c *Config) EffectiveThresholds() Thresholds {
	t := DefaultThresholds()
	if c == nil || c.Thresholds == nil {
		return t
	}
	if c.Thresholds.CautionCostRatio != nil {
		t.CautionCostRatio = *c.Thresholds.CautionCostRatio
	}
	if c.Thresholds.StopCostRatio != nil {
		t.StopCostRatio = *c.Thresholds.StopCostRatio
	}
	if c.Thresholds.MinCostDelta != nil {
		t.MinCostDelta = *c.Thresholds.MinCostDelta
	}
	if c.Thresholds.CautionRowsRatio != nil {
		t.CautionRowsRatio = *c.Thresholds.CautionRowsRatio
	}
	if c.Thresholds.StopRowsRatio != nil {
		t.StopRowsRatio = *c.Thresholds.StopRowsRatio
	}
	if c.Thresholds.MinRowsDelta != nil {
		t.MinRowsDelta = *c.Thresholds.MinRowsDelta
	}
	return t
}

// LoadConfig reads and validates a SchemaGuard config file.
//
// The committed config-absent behavior (docs/tasks.md 4.2):
//
//   - path == ""                     → returns (nil, nil). Plan
//                                      analysis is silently disabled;
//                                      the CLI still runs migration
//                                      and lock analysis.
//   - path provided, file unreadable → returns an error (exit 3).
//   - path provided, malformed YAML  → returns an error (exit 3).
//   - path provided, schema violation
//     (missing query id or sql)      → returns an error (exit 3).
//   - path provided, zero queries    → returns a valid *Config with
//                                      len(Queries) == 0. Plan
//                                      analysis is a no-op — no
//                                      findings are produced.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse YAML config: %w", err)
	}
	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

// validateConfig enforces the minimal schema rules: every query
// entry must have a non-empty ID and SQL, and IDs must be unique.
// Extra fields in the YAML are rejected by yaml.v3's strict decoder
// if it were enabled; we accept them silently to keep the schema
// forward-compatible.
func validateConfig(cfg *Config) error {
	seen := make(map[string]bool, len(cfg.Queries))
	for i, q := range cfg.Queries {
		if strings.TrimSpace(q.ID) == "" {
			return fmt.Errorf("queries[%d]: id is required", i)
		}
		if strings.TrimSpace(q.SQL) == "" {
			return fmt.Errorf("queries[%d] (id=%q): sql is required", i, q.ID)
		}
		if seen[q.ID] {
			return fmt.Errorf("queries[%d]: duplicate id %q", i, q.ID)
		}
		seen[q.ID] = true
	}
	if cfg.Thresholds != nil {
		if err := validateThresholdOverrides(cfg.Thresholds); err != nil {
			return err
		}
	}
	// Cross-field sanity on the EFFECTIVE thresholds (after merging
	// overrides with defaults). This catches the subtle case where
	// an override-only check would miss an inconsistency, e.g. a
	// user setting caution_cost_ratio to 6.0 while leaving the
	// default stop_cost_ratio of 5.0 in place.
	if err := validateEffectiveThresholds(cfg.EffectiveThresholds()); err != nil {
		return err
	}
	return nil
}

// validateThresholdOverrides rejects per-field threshold overrides
// that would make no sense mathematically — ratios below 1.0 (which
// would imply a plan is worse when it is actually better) and
// negative min-delta floors.
func validateThresholdOverrides(t *ThresholdOverrides) error {
	if t.CautionCostRatio != nil && *t.CautionCostRatio < 1.0 {
		return errors.New("thresholds.caution_cost_ratio must be >= 1.0")
	}
	if t.StopCostRatio != nil && *t.StopCostRatio < 1.0 {
		return errors.New("thresholds.stop_cost_ratio must be >= 1.0")
	}
	if t.CautionRowsRatio != nil && *t.CautionRowsRatio < 1.0 {
		return errors.New("thresholds.caution_rows_ratio must be >= 1.0")
	}
	if t.StopRowsRatio != nil && *t.StopRowsRatio < 1.0 {
		return errors.New("thresholds.stop_rows_ratio must be >= 1.0")
	}
	if t.MinCostDelta != nil && *t.MinCostDelta < 0 {
		return errors.New("thresholds.min_cost_delta must be >= 0")
	}
	if t.MinRowsDelta != nil && *t.MinRowsDelta < 0 {
		return errors.New("thresholds.min_rows_delta must be >= 0")
	}
	return nil
}

// validateEffectiveThresholds enforces consistency rules across the
// merged (default + override) threshold set. It runs after
// EffectiveThresholds has filled in any unset fields so the caller
// cannot accidentally configure a stop threshold that is actually
// looser than its caution threshold.
//
// These are the minimal sanity rules added in the pre-M5 hygiene
// pass (see docs/things_to_fix.md):
//
//   - stop_cost_ratio must be >= caution_cost_ratio
//   - stop_rows_ratio must be >= caution_rows_ratio
//   - min_cost_delta must be >= 0
//   - min_rows_delta must be >= 0
func validateEffectiveThresholds(t Thresholds) error {
	if t.StopCostRatio < t.CautionCostRatio {
		return fmt.Errorf("thresholds: effective stop_cost_ratio (%v) must be >= caution_cost_ratio (%v)",
			t.StopCostRatio, t.CautionCostRatio)
	}
	if t.StopRowsRatio < t.CautionRowsRatio {
		return fmt.Errorf("thresholds: effective stop_rows_ratio (%v) must be >= caution_rows_ratio (%v)",
			t.StopRowsRatio, t.CautionRowsRatio)
	}
	if t.MinCostDelta < 0 {
		return fmt.Errorf("thresholds: effective min_cost_delta (%v) must be >= 0", t.MinCostDelta)
	}
	if t.MinRowsDelta < 0 {
		return fmt.Errorf("thresholds: effective min_rows_delta (%v) must be >= 0", t.MinRowsDelta)
	}
	return nil
}
