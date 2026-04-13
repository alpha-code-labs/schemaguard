package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunNoArgsPrintsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"schemaguard"}, &stdout, &stderr)
	if code != ExitGreen {
		t.Fatalf("no args: expected ExitGreen (0), got %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage") {
		t.Errorf("no args: stdout did not contain Usage header: %q", stdout.String())
	}
}

func TestRunShortHelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"schemaguard", "-h"}, &stdout, &stderr)
	if code != ExitGreen {
		t.Fatalf("-h: expected ExitGreen (0), got %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "schemaguard") {
		t.Errorf("-h: stdout did not mention schemaguard: %q", stdout.String())
	}
}

func TestRunLongHelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"schemaguard", "--help"}, &stdout, &stderr)
	if code != ExitGreen {
		t.Fatalf("--help: expected ExitGreen (0), got %d", code)
	}
	if !strings.Contains(stdout.String(), "check") {
		t.Errorf("--help: stdout did not mention check subcommand: %q", stdout.String())
	}
}

func TestRunVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"schemaguard", "--version"}, &stdout, &stderr)
	if code != ExitGreen {
		t.Fatalf("--version: expected ExitGreen (0), got %d", code)
	}
	if !strings.Contains(stdout.String(), "schemaguard") {
		t.Errorf("--version: stdout did not mention schemaguard: %q", stdout.String())
	}
}

func TestRunVersionSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"schemaguard", "version"}, &stdout, &stderr)
	if code != ExitGreen {
		t.Fatalf("version: expected ExitGreen (0), got %d", code)
	}
	if !strings.Contains(stdout.String(), Version) {
		t.Errorf("version: stdout did not contain Version %q: %q", Version, stdout.String())
	}
}

func TestRunUnknownCommandReturnsToolError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"schemaguard", "nonsense"}, &stdout, &stderr)
	if code != ExitToolError {
		t.Fatalf("unknown command: expected ExitToolError (3), got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("unknown command: stderr missing 'unknown command': %q", stderr.String())
	}
}

func TestRunUnknownFlagReturnsToolError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"schemaguard", "--bogus"}, &stdout, &stderr)
	if code != ExitToolError {
		t.Fatalf("unknown flag: expected ExitToolError (3), got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown flag") {
		t.Errorf("unknown flag: stderr missing 'unknown flag': %q", stderr.String())
	}
}

func TestRunCheckHelpListsAllFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"schemaguard", "check", "-h"}, &stdout, &stderr)
	if code != ExitGreen {
		t.Fatalf("check -h: expected ExitGreen (0), got %d; stderr=%q", code, stderr.String())
	}
	help := stderr.String()
	requiredFlags := []string{
		"--migration",
		"--snapshot",
		"--config",
		"--dbt-manifest",
		"--format",
		"--out",
		"--verbose",
	}
	for _, f := range requiredFlags {
		if !strings.Contains(help, f) {
			t.Errorf("check -h: help did not mention %s: %q", f, help)
		}
	}
}

func TestRunCheckHelpMentionsExitCodes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	_ = Run([]string{"schemaguard", "check", "--help"}, &stdout, &stderr)
	help := stderr.String()
	for _, needle := range []string{"green", "yellow", "red", "tool error"} {
		if !strings.Contains(help, needle) {
			t.Errorf("check --help: expected help to mention %q: %q", needle, help)
		}
	}
}

func TestRunCheckWithoutMigrationFlagReturnsToolError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"schemaguard", "check",
		"--snapshot", "/nonexistent.dump",
	}, &stdout, &stderr)
	if code != ExitToolError {
		t.Fatalf("expected ExitToolError (3), got %d", code)
	}
	if !strings.Contains(stderr.String(), "--migration is required") {
		t.Errorf("stderr did not mention '--migration is required': %q", stderr.String())
	}
}

func TestRunCheckWithoutSnapshotFlagReturnsToolError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"schemaguard", "check",
		"--migration", "/nonexistent.sql",
	}, &stdout, &stderr)
	if code != ExitToolError {
		t.Fatalf("expected ExitToolError (3), got %d", code)
	}
	if !strings.Contains(stderr.String(), "--snapshot is required") {
		t.Errorf("stderr did not mention '--snapshot is required': %q", stderr.String())
	}
}

func TestRunCheckWithMissingMigrationFileReturnsToolError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"schemaguard", "check",
		"--migration", "/definitely/not/a/real/path/m.sql",
		"--snapshot", "/definitely/not/a/real/path/d.dump",
	}, &stdout, &stderr)
	if code != ExitToolError {
		t.Fatalf("expected ExitToolError (3), got %d", code)
	}
	if !strings.Contains(stderr.String(), "loading migration") {
		t.Errorf("stderr did not mention 'loading migration': %q", stderr.String())
	}
}

func TestRunCheckWithMissingSnapshotFileReturnsToolError(t *testing.T) {
	// Create a real migration file so the tool gets past the migration
	// check and fails specifically on the snapshot existence check.
	dir := t.TempDir()
	migPath := dir + "/m.sql"
	if err := os.WriteFile(migPath, []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{
		"schemaguard", "check",
		"--migration", migPath,
		"--snapshot", "/definitely/not/a/real/path/d.dump",
	}, &stdout, &stderr)
	if code != ExitToolError {
		t.Fatalf("expected ExitToolError (3), got %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "snapshot file") {
		t.Errorf("stderr did not mention 'snapshot file': %q", stderr.String())
	}
}
