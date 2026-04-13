package shadowdb

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// DockerUnavailableMessage is the exact, committed error message surfaced
// when Docker cannot be reached. Task 2.5 in docs/tasks.md requires that
// this message be defined and committed, not generated ad-hoc. Any change
// to this wording must be approved and recorded via DECISIONS.md.
const DockerUnavailableMessage = `Docker is required but unavailable.

SchemaGuard v1 is Docker-only — it provisions an ephemeral Postgres
container as the shadow database. External Postgres mode is deferred to
v1.5.

Please make sure that:
  1. Docker is installed (Docker Desktop, Colima, OrbStack, or equivalent).
  2. The Docker daemon is running.
  3. ` + "`docker version`" + ` succeeds from your shell.

Then re-run ` + "`schemaguard check`" + `.`

// ErrDockerUnavailable is returned by CheckDockerAvailable when the
// Docker CLI is missing or the daemon cannot be reached. Callers should
// surface this as an ExitToolError and print the message verbatim.
var ErrDockerUnavailable = errors.New("docker unavailable")

// CheckDockerAvailable verifies that a Docker CLI is installed and its
// daemon is reachable. It runs ` + "`docker version --format {{.Server.Version}}`" + ` and
// treats any non-zero exit or missing executable as unavailable.
//
// On failure, the returned error wraps ErrDockerUnavailable and its
// Error() string begins with DockerUnavailableMessage so the CLI can
// print a consistent, committed message.
func CheckDockerAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("%s\n\nUnderlying error: %s: %w", DockerUnavailableMessage, detail, ErrDockerUnavailable)
}
