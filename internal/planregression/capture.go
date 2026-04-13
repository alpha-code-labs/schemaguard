package planregression

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CaptureResult is the outcome of running EXPLAIN (FORMAT JSON) on
// one top query. Plan is nil when Err is non-nil.
type CaptureResult struct {
	QueryID string
	SQL     string
	Plan    planNode
	Err     error
}

// HasError reports whether the EXPLAIN call itself failed — for
// example because the query references a column that no longer
// exists post-migration.
func (r CaptureResult) HasError() bool { return r.Err != nil }

// Capture runs EXPLAIN (FORMAT JSON) on each query in queries,
// against the given pgx connection, and returns one CaptureResult
// per query in the same order.
//
// The analyzer uses plan-only EXPLAIN — NEVER EXPLAIN ANALYZE — per
// docs/DECISIONS.md. The committed invariant is that the top queries
// are never executed against the shadow database; only their plans
// are captured. This function does not under any circumstances run
// ANALYZE or any other form of plan that would execute the query.
//
// Errors from individual queries are captured in the corresponding
// CaptureResult.Err and do NOT stop the loop. A broken query is a
// product finding, not a tool error.
func Capture(ctx context.Context, conn *pgx.Conn, queries []TopQuery) []CaptureResult {
	out := make([]CaptureResult, 0, len(queries))
	for _, q := range queries {
		res := CaptureResult{QueryID: q.ID, SQL: q.SQL}
		plan, err := explainOne(ctx, conn, q.SQL)
		if err != nil {
			res.Err = err
		} else {
			res.Plan = plan
		}
		out = append(out, res)
	}
	return out
}

// explainOne runs a single plan-only EXPLAIN against conn and
// returns the parsed root plan node.
func explainOne(ctx context.Context, conn *pgx.Conn, sql string) (planNode, error) {
	// Plan-only EXPLAIN. The FORMAT JSON option returns one row
	// containing a JSON-encoded plan array. We never add ANALYZE.
	stmt := fmt.Sprintf("EXPLAIN (FORMAT JSON) %s", sql)
	var raw []byte
	if err := conn.QueryRow(ctx, stmt).Scan(&raw); err != nil {
		return nil, err
	}
	return parseExplainJSON(raw)
}
