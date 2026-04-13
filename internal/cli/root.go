// Package cli implements the schemaguard command-line interface: flag
// parsing, subcommand dispatch, exit codes, and signal handling. It is the
// single orchestration layer for the rest of the tool.
//
// Milestone 1 covers only the skeleton: Run, the check subcommand's flag
// parsing, exit codes, version, help, and a cleanup hook registry wired
// to SIGINT/SIGTERM. The check pipeline (shadow DB, migration executor,
// analyzers, report generator) is implemented in later milestones.
package cli

import (
	"fmt"
	"io"
	"strings"
)

// Run is the CLI entry point. It parses the top-level command, dispatches
// to the appropriate subcommand, and returns an exit code per the
// convention defined in exitcode.go.
//
// args must include the program name as args[0], matching os.Args.
// stdout and stderr are taken as parameters rather than hardcoded to
// os.Stdout / os.Stderr so Run can be driven directly from tests.
func Run(args []string, stdout, stderr io.Writer) int {
	ctx, stop := installSignalHandler()
	defer stop()

	// No subcommand → show help and exit green.
	if len(args) < 2 {
		printRootHelp(stdout)
		return ExitGreen
	}

	switch args[1] {
	case "help", "--help", "-h":
		printRootHelp(stdout)
		return ExitGreen

	case "version", "--version":
		fmt.Fprintf(stdout, "schemaguard %s\n", Version)
		return ExitGreen

	case "check":
		return runCheck(ctx, args[2:], stdout, stderr)

	default:
		if strings.HasPrefix(args[1], "-") {
			fmt.Fprintf(stderr, "error: unknown flag %q\n\n", args[1])
		} else {
			fmt.Fprintf(stderr, "error: unknown command %q\n\n", args[1])
		}
		printRootHelp(stderr)
		return ExitToolError
	}
}

func printRootHelp(w io.Writer) {
	fmt.Fprint(w, `schemaguard — verify Postgres migrations against production-like data before deployment

Usage:
  schemaguard <command> [flags]

Commands:
  check       Run a migration against a shadow database and report risks
  version     Show the tool version
  help        Show this help

Global flags:
  -h, --help      Show help
      --version   Show version

Run "schemaguard check --help" for details on the check command.

Documentation lives in docs/ — requirements.md, build_spec.md, tasks.md, DECISIONS.md.
`)
}
