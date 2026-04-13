package planregression

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "schemaguard.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadConfigEmptyPathReturnsNilNil(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config for empty path, got %+v", cfg)
	}
}

func TestLoadConfigMissingFileReturnsError(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/schemaguard.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Errorf("error should mention 'read config', got %q", err.Error())
	}
}

func TestLoadConfigMalformedYAMLReturnsError(t *testing.T) {
	path := writeConfig(t, "queries: [unclosed")
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parse YAML config") {
		t.Errorf("error should mention 'parse YAML config', got %q", err.Error())
	}
}

func TestLoadConfigMissingQueryIDIsSchemaViolation(t *testing.T) {
	path := writeConfig(t, `queries:
  - sql: SELECT 1
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("error should mention 'id is required', got %q", err.Error())
	}
}

func TestLoadConfigMissingQuerySQLIsSchemaViolation(t *testing.T) {
	path := writeConfig(t, `queries:
  - id: q1
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing sql, got nil")
	}
	if !strings.Contains(err.Error(), "sql is required") {
		t.Errorf("error should mention 'sql is required', got %q", err.Error())
	}
}

func TestLoadConfigDuplicateIDIsSchemaViolation(t *testing.T) {
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
  - id: q1
    sql: SELECT 2
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for duplicate id, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("error should mention 'duplicate id', got %q", err.Error())
	}
}

func TestLoadConfigZeroQueriesIsValidNoOp(t *testing.T) {
	path := writeConfig(t, "queries: []\n")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for zero queries")
	}
	if len(cfg.Queries) != 0 {
		t.Errorf("expected 0 queries, got %d", len(cfg.Queries))
	}
}

func TestLoadConfigValidBasicConfig(t *testing.T) {
	path := writeConfig(t, `queries:
  - id: orders_by_customer
    sql: SELECT * FROM orders WHERE customer_id = 1
  - id: status_count
    sql: SELECT status, count(*) FROM orders GROUP BY status
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(cfg.Queries))
	}
	if cfg.Queries[0].ID != "orders_by_customer" {
		t.Errorf("query 0 id = %q", cfg.Queries[0].ID)
	}
}

func TestLoadConfigWithThresholdOverrides(t *testing.T) {
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  caution_cost_ratio: 3.0
  stop_cost_ratio: 10.0
  min_cost_delta: 50
  caution_rows_ratio: 5.0
  stop_rows_ratio: 50.0
  min_rows_delta: 500
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	eff := cfg.EffectiveThresholds()
	if eff.CautionCostRatio != 3.0 {
		t.Errorf("CautionCostRatio override = %v", eff.CautionCostRatio)
	}
	if eff.StopCostRatio != 10.0 {
		t.Errorf("StopCostRatio override = %v", eff.StopCostRatio)
	}
	if eff.MinCostDelta != 50 {
		t.Errorf("MinCostDelta override = %v", eff.MinCostDelta)
	}
	if eff.CautionRowsRatio != 5.0 {
		t.Errorf("CautionRowsRatio override = %v", eff.CautionRowsRatio)
	}
}

func TestLoadConfigPartialThresholdOverridesUseDefaults(t *testing.T) {
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  caution_cost_ratio: 3.0
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	eff := cfg.EffectiveThresholds()
	defaults := DefaultThresholds()
	if eff.CautionCostRatio != 3.0 {
		t.Errorf("CautionCostRatio = %v, want 3.0 (override)", eff.CautionCostRatio)
	}
	if eff.StopCostRatio != defaults.StopCostRatio {
		t.Errorf("StopCostRatio = %v, want default %v", eff.StopCostRatio, defaults.StopCostRatio)
	}
	if eff.CautionRowsRatio != defaults.CautionRowsRatio {
		t.Errorf("CautionRowsRatio = %v, want default %v", eff.CautionRowsRatio, defaults.CautionRowsRatio)
	}
}

func TestLoadConfigInvalidThresholdRatioRejected(t *testing.T) {
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  caution_cost_ratio: 0.5
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for ratio < 1.0, got nil")
	}
}

func TestLoadConfigNegativeMinCostDeltaRejected(t *testing.T) {
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  min_cost_delta: -10
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for negative min_cost_delta, got nil")
	}
	if !strings.Contains(err.Error(), "min_cost_delta") {
		t.Errorf("error should mention min_cost_delta, got %q", err.Error())
	}
}

func TestLoadConfigNegativeMinRowsDeltaRejected(t *testing.T) {
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  min_rows_delta: -500
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for negative min_rows_delta, got nil")
	}
	if !strings.Contains(err.Error(), "min_rows_delta") {
		t.Errorf("error should mention min_rows_delta, got %q", err.Error())
	}
}

func TestLoadConfigStopLessThanCautionCostRatioRejected(t *testing.T) {
	// stop=3 but caution=5 → stop < caution, invalid after merge.
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  caution_cost_ratio: 5.0
  stop_cost_ratio: 3.0
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for stop_cost_ratio < caution_cost_ratio, got nil")
	}
	if !strings.Contains(err.Error(), "stop_cost_ratio") {
		t.Errorf("error should mention stop_cost_ratio, got %q", err.Error())
	}
}

func TestLoadConfigStopLessThanCautionRowsRatioRejected(t *testing.T) {
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  caution_rows_ratio: 50.0
  stop_rows_ratio: 20.0
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for stop_rows_ratio < caution_rows_ratio, got nil")
	}
	if !strings.Contains(err.Error(), "stop_rows_ratio") {
		t.Errorf("error should mention stop_rows_ratio, got %q", err.Error())
	}
}

func TestLoadConfigCautionOverrideAboveDefaultStopIsRejected(t *testing.T) {
	// Default stop_cost_ratio is 5.0. Setting caution to 6.0 without
	// also raising stop would produce an inconsistent effective
	// threshold set — catch it via the cross-field check on the
	// merged values.
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  caution_cost_ratio: 6.0
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for caution override that exceeds default stop, got nil")
	}
	if !strings.Contains(err.Error(), "effective stop_cost_ratio") {
		t.Errorf("error should mention effective stop_cost_ratio, got %q", err.Error())
	}
}

func TestLoadConfigZeroMinDeltasAccepted(t *testing.T) {
	// Zero is the documented "flag every tiny change" value and
	// must remain accepted — only negatives are rejected.
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  min_cost_delta: 0
  min_rows_delta: 0
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error for zero min deltas: %v", err)
	}
	eff := cfg.EffectiveThresholds()
	if eff.MinCostDelta != 0 {
		t.Errorf("MinCostDelta = %v, want 0", eff.MinCostDelta)
	}
	if eff.MinRowsDelta != 0 {
		t.Errorf("MinRowsDelta = %v, want 0", eff.MinRowsDelta)
	}
}

func TestLoadConfigEqualCautionAndStopRatiosAccepted(t *testing.T) {
	// stop == caution is allowed (the analyzer will prefer the
	// stop branch in a tie, which is the conservative choice).
	path := writeConfig(t, `queries:
  - id: q1
    sql: SELECT 1
thresholds:
  caution_cost_ratio: 3.0
  stop_cost_ratio: 3.0
`)
	if _, err := LoadConfig(path); err != nil {
		t.Errorf("expected stop == caution to be accepted, got %v", err)
	}
}

func TestEffectiveThresholdsOnNilConfig(t *testing.T) {
	var cfg *Config
	eff := cfg.EffectiveThresholds()
	defaults := DefaultThresholds()
	if eff != defaults {
		t.Errorf("nil config should return defaults, got %+v", eff)
	}
}

func TestDefaultThresholdsMatchesDecision(t *testing.T) {
	// Guards the M4 decision task 4.6. Any change requires updating
	// docs/DECISIONS.md in lockstep.
	got := DefaultThresholds()
	want := Thresholds{
		CautionCostRatio: 2.0,
		StopCostRatio:    5.0,
		MinCostDelta:     100.0,
		CautionRowsRatio: 10.0,
		StopRowsRatio:    100.0,
		MinRowsDelta:     1000.0,
	}
	if got != want {
		t.Errorf("DefaultThresholds() = %+v, want %+v (see docs/DECISIONS.md)", got, want)
	}
}
