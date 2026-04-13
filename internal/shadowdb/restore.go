package shadowdb

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RestoreResult describes the outcome of RestoreSnapshot. Duration is
// the wall-clock time the restore took, rounded to the nearest ms by
// callers when printing.
type RestoreResult struct {
	Duration time.Duration
	Format   string // "sql", "custom", or "tar"
}

// RestoreSnapshot restores the user's dump file into the running shadow
// Postgres. The file must already be bind-mounted at containerSnapshotPath
// inside the container (which NewRunner + Start take care of).
//
// The format is chosen from the file extension:
//
//	.sql         → piped into psql
//	.dump        → pg_restore (custom format)
//	.tar         → pg_restore -F t
//	.pgdump      → pg_restore (custom format)
//
// Unknown extensions are rejected before touching the container.
//
// Errors from psql / pg_restore include the combined output from the
// subprocess so the user can see which statement failed during restore.
func (r *Runner) RestoreSnapshot(ctx context.Context) (*RestoreResult, error) {
	r.mu.Lock()
	started := r.started
	r.mu.Unlock()
	if !started {
		return nil, fmt.Errorf("runner not started")
	}

	ext := strings.ToLower(filepath.Ext(r.dumpPath))
	var (
		format string
		args   []string
	)
	switch ext {
	case ".sql":
		format = "sql"
		args = []string{
			"exec", r.name, "psql",
			"-U", "postgres",
			"-d", "postgres",
			"-v", "ON_ERROR_STOP=1",
			"-X", "-q",
			"-f", containerSnapshotPath,
		}
	case ".dump", ".pgdump":
		format = "custom"
		args = []string{
			"exec", r.name, "pg_restore",
			"-U", "postgres",
			"-d", "postgres",
			"--no-owner", "--no-privileges",
			containerSnapshotPath,
		}
	case ".tar":
		format = "tar"
		args = []string{
			"exec", r.name, "pg_restore",
			"-U", "postgres",
			"-d", "postgres",
			"-F", "t",
			"--no-owner", "--no-privileges",
			containerSnapshotPath,
		}
	default:
		return nil, fmt.Errorf("unsupported snapshot format %q (supported: .sql, .dump, .tar)", ext)
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("snapshot restore (%s) failed: %w\n%s", format, err, strings.TrimSpace(string(out)))
	}

	return &RestoreResult{
		Duration: elapsed,
		Format:   format,
	}, nil
}
