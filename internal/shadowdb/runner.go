package shadowdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// defaultImage is the Postgres Docker image SchemaGuard provisions as the
// shadow database. It is small, widely available, and matches the v1
// Docker-only rule in docs/DECISIONS.md.
const defaultImage = "postgres:16-alpine"

// containerSnapshotPath is where the user's dump file is bind-mounted
// inside the shadow container. Using a fixed path keeps the snapshot
// restore commands simple and consistent regardless of the host path.
const containerSnapshotPath = "/tmp/schemaguard-snapshot"

// readinessTimeout bounds how long Start() waits for the shadow Postgres
// to answer pg_isready before giving up.
const readinessTimeout = 45 * time.Second

// Runner owns the lifecycle of one ephemeral Postgres shadow container.
// A Runner is started once, used for one migration check, and then
// stopped. It is not reusable after Stop.
type Runner struct {
	name     string
	image    string
	dumpPath string

	mu       sync.Mutex
	started  bool
	stopped  bool
	hostPort int
}

// NewRunner creates a Runner configured to bind-mount dumpPath into the
// shadow container. dumpPath must be an absolute path that already exists
// on the host. The container itself is not started until Start is called.
func NewRunner(dumpPath string) (*Runner, error) {
	if dumpPath == "" {
		return nil, errors.New("snapshot path is required")
	}
	if !filepath.IsAbs(dumpPath) {
		return nil, fmt.Errorf("snapshot path must be absolute, got %q", dumpPath)
	}
	suffix, err := randomSuffix(6)
	if err != nil {
		return nil, fmt.Errorf("generate container name: %w", err)
	}
	return &Runner{
		name:     "schemaguard-shadow-" + suffix,
		image:    defaultImage,
		dumpPath: dumpPath,
	}, nil
}

// Name returns the Docker container name assigned to this runner.
func (r *Runner) Name() string { return r.name }

// Image returns the Docker image used for the shadow container.
func (r *Runner) Image() string { return r.image }

// ConnString returns a libpq-style URL pointing at the shadow Postgres
// over the mapped host port. Callers should only use this after Start
// returns successfully.
func (r *Runner) ConnString() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fmt.Sprintf("postgres://postgres@127.0.0.1:%d/postgres?sslmode=disable", r.hostPort)
}

// Start provisions the shadow Postgres container: it runs the image with
// the snapshot file bind-mounted read-only, auto-publishes the Postgres
// port to a random host port, and waits for pg_isready to succeed.
//
// Start does not restore the snapshot — callers must invoke
// RestoreSnapshot after Start for the user's data to be available. This
// two-step split keeps start/readiness timing measurable separately from
// restore timing.
//
// If Start fails partway (e.g. the container started but readiness
// never succeeded), Start attempts a best-effort teardown before
// returning so the caller never sees a half-running container.
func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return errors.New("runner already started")
	}
	r.started = true
	r.mu.Unlock()

	runArgs := []string{
		"run", "-d", "--rm",
		"--name", r.name,
		"-P",
		"-e", "POSTGRES_HOST_AUTH_METHOD=trust",
		"-v", fmt.Sprintf("%s:%s:ro", r.dumpPath, containerSnapshotPath),
		r.image,
	}
	if out, err := exec.CommandContext(ctx, "docker", runArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("docker run: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	port, err := discoverHostPort(ctx, r.name)
	if err != nil {
		_ = r.forceStop(context.Background())
		return fmt.Errorf("discover host port: %w", err)
	}
	r.mu.Lock()
	r.hostPort = port
	r.mu.Unlock()

	if err := r.waitReady(ctx); err != nil {
		_ = r.forceStop(context.Background())
		return fmt.Errorf("shadow DB readiness: %w", err)
	}
	return nil
}

// Stop tears down the shadow container. It is safe to call more than
// once; subsequent calls are no-ops. Stop is intentionally tolerant of
// errors — if the container is already gone, Stop returns nil.
//
// Stop uses context.Background for the actual docker commands so that
// cleanup paths triggered by a cancelled parent context still manage to
// remove the container.
func (r *Runner) Stop(_ context.Context) error {
	r.mu.Lock()
	if r.stopped || !r.started {
		r.stopped = true
		r.mu.Unlock()
		return nil
	}
	r.stopped = true
	r.mu.Unlock()
	return r.forceStop(context.Background())
}

// forceStop removes the container regardless of current state. It is
// used both by Stop and by failure paths inside Start.
func (r *Runner) forceStop(ctx context.Context) error {
	// `docker stop` with a short grace period; --rm removes the
	// container once it exits.
	stopCmd := exec.CommandContext(ctx, "docker", "stop", "-t", "3", r.name)
	stopOut, stopErr := stopCmd.CombinedOutput()

	if stopErr != nil {
		// Best-effort kill and remove in case stop failed because the
		// container was already half-gone or unresponsive.
		_ = exec.CommandContext(ctx, "docker", "kill", r.name).Run()
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", r.name).Run()
	}
	_ = stopOut
	_ = stopErr
	return nil
}

func (r *Runner) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(readinessTimeout)
	for {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		cmd := exec.CommandContext(probeCtx, "docker", "exec", r.name,
			"pg_isready", "-U", "postgres", "-d", "postgres", "-h", "127.0.0.1")
		err := cmd.Run()
		cancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for shadow DB", readinessTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// discoverHostPort asks Docker which host port was auto-mapped to the
// container's 5432/tcp and parses the result.
func discoverHostPort(ctx context.Context, name string) (int, error) {
	out, err := exec.CommandContext(ctx, "docker", "port", name, "5432/tcp").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("docker port: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return parseDockerPort(string(out))
}

// parseDockerPort extracts a host port from ` + "`docker port <name> 5432/tcp`" + `
// output. The command typically prints lines like:
//
//	0.0.0.0:55842
//	[::]:55842
//
// The function returns the first port it successfully parses.
func parseDockerPort(out string) (int, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ":")
		if idx < 0 {
			continue
		}
		port, err := strconv.Atoi(line[idx+1:])
		if err != nil {
			continue
		}
		if port > 0 {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no host port found in docker port output: %q", strings.TrimSpace(out))
}

// randomSuffix produces a short hex-encoded random suffix for unique
// container names. A length of 6 bytes yields 12 hex characters — enough
// entropy to avoid collisions across concurrent runs without being ugly.
func randomSuffix(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
