package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/alpha-code-labs/schemaguard/internal/executor"
	"github.com/alpha-code-labs/schemaguard/internal/lockanalyzer"
	"github.com/alpha-code-labs/schemaguard/internal/planregression"
	"github.com/alpha-code-labs/schemaguard/internal/report"
	"github.com/alpha-code-labs/schemaguard/internal/shadowdb"
)

// runCheck is the `schemaguard check` subcommand entry point. It parses
// flags, runs the full M1–M5 pipeline (shadow DB provisioning →
// snapshot restore → optional baseline plan capture → migration
// execution with concurrent lock sampling → optional post-migration
// plan capture → analysis → unified report), and writes the report to
// stdout (or --out) in the requested format.
//
// Progress messages go to stderr so that piping the report (for
// example, `schemaguard check --format json > report.json`) produces
// a clean report file with status lines still visible on the
// terminal.
//
// Exit codes are computed from the unified verdict in internal/report
// and match docs/DECISIONS.md:
//
//	0 green       — no significant findings
//	1 yellow      — caution-level findings only
//	2 red         — stop-level findings OR a halted migration
//	3 tool error  — bad input, Docker unavailable, restore failure, crash
func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	runStart := time.Now()

	fs := flag.NewFlagSet("schemaguard check", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		migration   string
		snapshot    string
		configPath  string
		dbtManifest string
		format      string
		outPath     string
		verbose     bool
	)

	fs.StringVar(&migration, "migration", "",
		"Path to the migration SQL file or directory (required)")
	fs.StringVar(&snapshot, "snapshot", "",
		"Path to a Postgres dump file (.sql, .dump, or .tar) that will be restored into an ephemeral Docker-based shadow database (required)")
	fs.StringVar(&configPath, "config", "",
		"Path to a YAML config file with top queries and plan-regression threshold overrides (optional — when absent, plan analysis is silently disabled)")
	fs.StringVar(&format, "format", "text",
		"Output format for the report: text, json, or markdown (default: text)")
	fs.StringVar(&outPath, "out", "",
		"Write the report to a file instead of stdout (optional)")
	// The two flags below remain inert in M5 and are scheduled for
	// later milestones. Each description ends with the milestone
	// that will wire it up.
	fs.StringVar(&dbtManifest, "dbt-manifest", "",
		"Path to a dbt manifest.json (NOT YET IMPLEMENTED — wired in Milestone 9 only if the secondary dbt capability ships)")
	fs.BoolVar(&verbose, "verbose", false,
		"Extra logging for debugging runs (NOT YET IMPLEMENTED — wired in a later milestone)")

	fs.Usage = func() {
		fmt.Fprint(stderr, `schemaguard check — verify a Postgres migration against a shadow database

Usage:
  schemaguard check --migration <path> --snapshot <path> [flags]

Required:
      --migration <path>       Path to the migration SQL file or directory
      --snapshot <path>        Path to a Postgres dump file (.sql, .dump, or
                               .tar) that will be restored into an ephemeral
                               Docker-based shadow database

Optional:
      --config <path>          YAML config with top queries and plan-
                               regression threshold overrides. When
                               absent, plan analysis is silently
                               disabled; the tool still runs migration
                               and lock analysis.
      --format <fmt>           Output format: text, json, or markdown
                               (default: text).
      --out <path>             Write the report to a file instead of
                               stdout. Progress messages continue to
                               stream to stderr.

Not yet implemented (parsed today to avoid "unknown flag" errors; will
be wired up by the milestones in brackets):
      --dbt-manifest <path>    Path to a dbt manifest.json        [M9]
      --verbose                Extra logging for debugging runs   [later]
  -h, --help                   Show this help

Exit codes:
  0  green       — no significant findings
  1  yellow      — caution-level findings, merge with care
  2  red         — stop-level findings, do not merge (includes a migration
                   that halted on its own SQL error)
  3  tool error  — bad inputs, snapshot restore failure, Docker unavailable,
                   or internal crash

See docs/build_spec.md for the full CLI behavior.
`)
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return ExitGreen
		}
		return ExitToolError
	}

	// INERT FLAGS — DO NOT WIRE UP HERE.
	//
	// The two flags below are parsed only so users experimenting
	// with the CLI do not hit "unknown flag" errors. They are
	// deliberately not connected to any behavior in Milestones 1–5.
	// Each is owned by a later milestone and MUST be implemented in
	// that milestone's code path, not here, so scope stays honest:
	//
	//   --dbt-manifest  → Milestone 9 only (secondary dbt capability;
	//                     may never ship in v1 — see docs/DECISIONS.md)
	//   --verbose       → a later milestone (per-stage logging)
	//
	// Note: --config moved OUT of this list in M4, and --format /
	// --out moved OUT in M5. They are all wired up below.
	_ = dbtManifest
	_ = verbose

	if migration == "" {
		fmt.Fprintln(stderr, "error: --migration is required")
		return ExitToolError
	}
	if snapshot == "" {
		fmt.Fprintln(stderr, "error: --snapshot is required")
		return ExitToolError
	}
	if err := validateFormat(format); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitToolError
	}

	migrationSQL, err := loadMigration(migration)
	if err != nil {
		fmt.Fprintf(stderr, "error: loading migration: %v\n", err)
		return ExitToolError
	}

	// Load the plan-regression config. LoadConfig implements the
	// committed config-absent / malformed / empty-queries behavior
	// from docs/tasks.md 4.2. A nil planCfg means "plan analysis is
	// silently disabled"; an error means "tool failure, exit 3".
	planCfg, err := planregression.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: loading config: %v\n", err)
		return ExitToolError
	}

	absSnapshot, err := filepath.Abs(snapshot)
	if err != nil {
		fmt.Fprintf(stderr, "error: resolving snapshot path: %v\n", err)
		return ExitToolError
	}
	if info, err := os.Stat(absSnapshot); err != nil {
		fmt.Fprintf(stderr, "error: snapshot file: %v\n", err)
		return ExitToolError
	} else if info.IsDir() {
		fmt.Fprintf(stderr, "error: snapshot path %q is a directory, expected a dump file\n", absSnapshot)
		return ExitToolError
	}

	if err := shadowdb.CheckDockerAvailable(ctx); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitToolError
	}

	runner, err := shadowdb.NewRunner(absSnapshot)
	if err != nil {
		fmt.Fprintf(stderr, "error: shadow DB runner: %v\n", err)
		return ExitToolError
	}

	// Register teardown immediately so a signal after `docker run`
	// succeeds but before Start returns still removes the container.
	RegisterCleanup(func() {
		_ = runner.Stop(context.Background())
	})

	fmt.Fprintln(stderr, "==> Starting shadow Postgres...")
	startBegin := time.Now()
	if err := runner.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "error: starting shadow DB: %v\n", err)
		return ExitToolError
	}
	fmt.Fprintf(stderr, "    ready in %s (%s)\n",
		time.Since(startBegin).Round(time.Millisecond), runner.Name())

	fmt.Fprintln(stderr, "==> Restoring snapshot...")
	restoreResult, err := runner.RestoreSnapshot(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "error: restoring snapshot: %v\n", err)
		return ExitToolError
	}
	fmt.Fprintf(stderr, "    done in %s (format: %s)\n",
		restoreResult.Duration.Round(time.Millisecond), restoreResult.Format)

	conn, err := pgx.Connect(ctx, runner.ConnString())
	if err != nil {
		fmt.Fprintf(stderr, "error: connecting to shadow DB: %v\n", err)
		return ExitToolError
	}
	defer conn.Close(context.Background())

	// Second pgx connection dedicated to the lock sampler. It must
	// not share with the migration connection or the sampler will
	// serialize behind the migration's transaction.
	samplerConn, err := pgx.Connect(ctx, runner.ConnString())
	if err != nil {
		fmt.Fprintf(stderr, "error: connecting sampler to shadow DB: %v\n", err)
		return ExitToolError
	}
	defer samplerConn.Close(context.Background())

	// M5: capture the restored shadow DB size for the report footer.
	// This is a single cheap query; errors are non-fatal.
	var shadowDBSizeBytes int64
	_ = conn.QueryRow(ctx, "SELECT pg_database_size('postgres')").Scan(&shadowDBSizeBytes)

	// M4: baseline plan capture. Runs BEFORE the sampler starts so
	// the sampler does not see EXPLAIN's own AccessShare locks and
	// attribute them to phantom statements.
	var baselinePlans []planregression.CaptureResult
	planEnabled := planCfg != nil && len(planCfg.Queries) > 0
	if planEnabled {
		fmt.Fprintln(stderr, "==> Capturing baseline query plans...")
		baselinePlans = planregression.Capture(ctx, conn, planCfg.Queries)
	}

	migrationPID := conn.PgConn().PID()
	sampler := lockanalyzer.NewSampler(samplerConn, migrationPID, lockanalyzer.DefaultSamplingInterval)
	samplerCtx, cancelSampler := context.WithCancel(ctx)
	go sampler.Run(samplerCtx)

	fmt.Fprintln(stderr, "==> Running migration...")
	result, runErr := executor.Run(ctx, conn, migrationSQL)

	// Stop the sampler cleanly regardless of success or failure. Per
	// tasks.md 3.1, samples collected before a mid-migration failure
	// must still be analyzable.
	cancelSampler()
	<-sampler.Done()

	if runErr != nil {
		fmt.Fprintf(stderr, "error: running migration: %v\n", runErr)
		return ExitToolError
	}

	// M4: post-migration plan capture. Per docs/DECISIONS.md and
	// tasks.md 4.4 this step is SKIPPED when the migration halted on
	// its own SQL error — there is no meaningful post-state to
	// EXPLAIN against.
	var postPlans []planregression.CaptureResult
	var planFindings []planregression.Finding
	if planEnabled && !result.Failed {
		fmt.Fprintln(stderr, "==> Capturing post-migration query plans...")
		postPlans = planregression.Capture(ctx, conn, planCfg.Queries)
		planFindings = planregression.Analyze(baselinePlans, postPlans, planCfg.EffectiveThresholds())
	}

	lockFindings := lockanalyzer.Analyze(
		sampler.Samples(),
		statementWindowsFromResult(result),
		lockanalyzer.DefaultSamplingInterval,
	)

	// M5: build the unified report and write it to the requested
	// destination in the requested format. This replaces the
	// temporary M2+ plumbing output from Milestones 2–4.
	rep := report.Build(report.Input{
		ToolVersion:       Version,
		MigrationResult:   result,
		LockFindings:      lockFindings,
		PlanFindings:      planFindings,
		RunDuration:       time.Since(runStart),
		RestoreDuration:   restoreResult.Duration,
		ShadowDBImage:     runner.Image(),
		ShadowDBSizeBytes: shadowDBSizeBytes,
	})

	if err := writeReport(&rep, format, outPath, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "error: writing report: %v\n", err)
		return ExitToolError
	}

	return rep.Verdict.ExitCode()
}

// validateFormat rejects --format values the report package does
// not recognize. The three supported formats mirror the M5
// deliverables in docs/tasks.md 5.3–5.5.
func validateFormat(f string) error {
	switch f {
	case "text", "json", "markdown":
		return nil
	}
	return fmt.Errorf("unknown --format %q (supported: text, json, markdown)", f)
}

// writeReport formats the report and writes it to --out or stdout.
// Progress messages are never written here — they go to stderr
// directly from runCheck. The report writer is purely "format +
// emit bytes."
func writeReport(rep *report.Report, format, outPath string, stdout, stderr io.Writer) error {
	var body []byte
	switch format {
	case "json":
		b, err := report.FormatJSON(rep)
		if err != nil {
			return fmt.Errorf("json format: %w", err)
		}
		body = b
	case "markdown":
		body = []byte(report.FormatMarkdown(rep))
	default:
		body = []byte(report.FormatText(rep))
	}

	if outPath == "" {
		_, err := stdout.Write(body)
		return err
	}
	// Write to the requested file. Use 0o644 so reviewers can
	// inspect the report by hand without extra chmod steps.
	if err := os.WriteFile(outPath, body, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "==> Report written to %s\n", outPath)
	return nil
}

// statementWindowsFromResult converts executor.StatementResult values
// into the minimal StatementWindow slice the lock analyzer needs,
// keeping internal/lockanalyzer independent of internal/executor.
func statementWindowsFromResult(result *executor.Result) []lockanalyzer.StatementWindow {
	if result == nil {
		return nil
	}
	out := make([]lockanalyzer.StatementWindow, 0, len(result.Statements))
	for _, s := range result.Statements {
		out = append(out, lockanalyzer.StatementWindow{
			Index:     s.Index,
			StartedAt: s.StartedAt,
			EndAt:     s.EndAt(),
			SQL:       s.SQL,
		})
	}
	return out
}

// loadMigration reads a migration file or directory of .sql files and
// returns the concatenated SQL. Directories are walked in lexical order.
// Entries that are not .sql files are skipped.
func loadMigration(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	var combined string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(path, entry.Name()))
		if err != nil {
			return "", err
		}
		combined += "\n-- schemaguard: " + entry.Name() + "\n"
		combined += string(b)
		combined += "\n"
	}
	if combined == "" {
		return "", errors.New("no .sql files found in migration directory")
	}
	return combined, nil
}
