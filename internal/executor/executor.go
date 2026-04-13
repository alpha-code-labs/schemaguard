package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// execer is the tiny slice of pgx.Conn / pgx.Tx that the executor needs.
// It exists so runStatements can drive either a transaction handle or a
// bare connection without branching.
type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Run executes the migration script against conn and returns a
// structured Result describing every statement's outcome.
//
// By default the entire migration is wrapped in a single transaction.
// If the migration script contains its own BEGIN/COMMIT/ROLLBACK/END or
// START TRANSACTION statements, Run respects them instead of wrapping —
// this matches how Flyway and Alembic execute Postgres migrations and
// is the closed decision in docs/DECISIONS.md.
//
// Run stops at the first statement error and returns a Result with
// Failed=true and FailureIndex / FailureErr populated. It does not
// return a Go error for SQL errors — those are product findings. A
// non-nil Go error return is reserved for unusual conditions like a
// failure to open the transaction or the context being cancelled before
// any statement runs.
func Run(ctx context.Context, conn *pgx.Conn, migrationSQL string) (*Result, error) {
	if conn == nil {
		return nil, errors.New("nil connection")
	}

	stmts := SplitStatements(migrationSQL)
	explicit := HasExplicitTransaction(stmts)

	result := &Result{
		Statements:   make([]StatementResult, 0, len(stmts)),
		ExplicitTx:   explicit,
		FailureIndex: -1,
	}

	if len(stmts) == 0 {
		return result, nil
	}

	start := time.Now()
	defer func() { result.TotalDuration = time.Since(start) }()

	if explicit {
		runStatements(ctx, conn, stmts, result)
		return result, nil
	}
	return runWrapped(ctx, conn, stmts, result)
}

// runWrapped runs the migration inside an implicit transaction. On any
// statement error, it rolls back and marks the result failed. If the
// entire statement stream succeeds, it commits; commit failures are
// recorded as migration failures (Failed=true, FailureIndex=-1).
func runWrapped(ctx context.Context, conn *pgx.Conn, stmts []string, result *Result) (*Result, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	runStatements(ctx, tx, stmts, result)

	if result.Failed {
		// Rollback errors are intentionally swallowed — the original
		// migration error is what the user needs to see.
		_ = tx.Rollback(ctx)
		return result, nil
	}

	if err := tx.Commit(ctx); err != nil {
		result.Failed = true
		result.FailureErr = fmt.Errorf("commit transaction: %w", err)
		return result, nil
	}
	return result, nil
}

// runStatements walks stmts in order, times each Exec call, and stops
// on the first error. It mutates result in place.
func runStatements(ctx context.Context, e execer, stmts []string, result *Result) {
	for i, stmt := range stmts {
		select {
		case <-ctx.Done():
			result.Failed = true
			result.FailureIndex = i
			result.FailureErr = ctx.Err()
			result.Statements = append(result.Statements, StatementResult{
				Index: i,
				SQL:   stmt,
				Err:   ctx.Err(),
			})
			return
		default:
		}

		s := execOne(ctx, e, i, stmt)
		result.Statements = append(result.Statements, s)
		if s.Err != nil {
			result.Failed = true
			result.FailureIndex = i
			result.FailureErr = s.Err
			return
		}
	}
}

func execOne(ctx context.Context, e execer, i int, stmt string) StatementResult {
	start := time.Now()
	_, err := e.Exec(ctx, stmt)
	return StatementResult{
		Index:     i,
		SQL:       stmt,
		StartedAt: start,
		Duration:  time.Since(start),
		Err:       err,
	}
}
