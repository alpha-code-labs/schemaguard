#!/usr/bin/env bash
#
# SchemaGuard GitHub Action entrypoint.
#
# A thin orchestration wrapper around the SchemaGuard CLI. It
# validates inputs, resolves the snapshot to an absolute local path
# (downloading from snapshot-url if that branch is chosen), invokes
# `schemaguard check --format markdown --out <tmp>`, unconditionally
# logs the rendered report to the Action log, upserts a PR comment
# carrying the report, and maps the CLI exit code to a step status
# with a distinct annotation per exit-code class.
#
# Per docs/DECISIONS.md the Action must contain NO detection logic.
# Everything in this file is orchestration: input resolution,
# subprocess invocation, output logging, comment upsert, exit-code
# mapping. The CLI remains the authoritative binary for every
# product decision.
#
# Inputs are read from environment variables populated by action.yml:
#   INPUT_MIGRATION      required
#   INPUT_SNAPSHOT_PATH  mutually exclusive with INPUT_SNAPSHOT_URL
#   INPUT_SNAPSHOT_URL   mutually exclusive with INPUT_SNAPSHOT_PATH
#   INPUT_CONFIG         optional
#
# Required GitHub runner env (populated automatically):
#   GITHUB_ACTION_PATH, GITHUB_WORKSPACE, RUNNER_TEMP,
#   GITHUB_EVENT_NAME, GITHUB_EVENT_PATH, GITHUB_REPOSITORY,
#   GITHUB_OUTPUT, GITHUB_TOKEN (for comment upsert).

set -uo pipefail

ACTION_DIR="${GITHUB_ACTION_PATH}/action"
SCHEMAGUARD_BIN="${RUNNER_TEMP}/schemaguard"
REPORT_PATH="${RUNNER_TEMP}/schemaguard-report.md"

MIGRATION_INPUT="${INPUT_MIGRATION:-}"
SNAPSHOT_PATH_INPUT="${INPUT_SNAPSHOT_PATH:-}"
SNAPSHOT_URL_INPUT="${INPUT_SNAPSHOT_URL:-}"
CONFIG_INPUT="${INPUT_CONFIG:-}"

# --- Input validation ------------------------------------------------------

if [[ -z "$MIGRATION_INPUT" ]]; then
  echo "::error::schemaguard-action: 'migration' input is required"
  exit 1
fi

if [[ -z "$SNAPSHOT_PATH_INPUT" && -z "$SNAPSHOT_URL_INPUT" ]]; then
  echo "::error::schemaguard-action: exactly one of 'snapshot-path' or 'snapshot-url' must be set"
  exit 1
fi
if [[ -n "$SNAPSHOT_PATH_INPUT" && -n "$SNAPSHOT_URL_INPUT" ]]; then
  echo "::error::schemaguard-action: 'snapshot-path' and 'snapshot-url' are mutually exclusive — set exactly one"
  exit 1
fi

if [[ ! -x "$SCHEMAGUARD_BIN" && ! -f "$SCHEMAGUARD_BIN" ]]; then
  echo "::error::schemaguard-action: schemaguard binary not found at $SCHEMAGUARD_BIN (the Build step must run first)"
  exit 1
fi

# --- Snapshot resolution ---------------------------------------------------

SNAPSHOT_FILE=""
if [[ -n "$SNAPSHOT_PATH_INPUT" ]]; then
  SNAPSHOT_FILE="${GITHUB_WORKSPACE}/${SNAPSHOT_PATH_INPUT}"
  if [[ ! -f "$SNAPSHOT_FILE" ]]; then
    echo "::error::schemaguard-action: snapshot-path '$SNAPSHOT_PATH_INPUT' does not exist in the PR checkout (resolved to '$SNAPSHOT_FILE')"
    exit 1
  fi
  echo "==> Using repo-relative snapshot: $SNAPSHOT_FILE"
else
  # Preserve the URL's extension so the CLI's extension-based format
  # dispatcher (.sql / .dump / .tar / .pgdump) still works.
  URL_EXT="${SNAPSHOT_URL_INPUT##*.}"
  case "$URL_EXT" in
    sql|dump|tar|pgdump) ;;
    *)
      echo "::warning::schemaguard-action: snapshot-url has no recognized extension; assuming .sql"
      URL_EXT="sql"
      ;;
  esac
  SNAPSHOT_FILE="${RUNNER_TEMP}/schemaguard-snapshot.${URL_EXT}"
  echo "==> Downloading snapshot from $SNAPSHOT_URL_INPUT (timeout 600s)"
  if ! curl --fail --silent --show-error --location --max-time 600 \
       --output "$SNAPSHOT_FILE" "$SNAPSHOT_URL_INPUT"; then
    echo "::error::schemaguard-action: failed to download snapshot from $SNAPSHOT_URL_INPUT"
    exit 1
  fi
  SNAPSHOT_SIZE=$(du -h "$SNAPSHOT_FILE" 2>/dev/null | cut -f1 || echo "?")
  echo "    downloaded ${SNAPSHOT_SIZE} to $SNAPSHOT_FILE"
fi

# --- Migration and config resolution ---------------------------------------

MIGRATION_FILE="${GITHUB_WORKSPACE}/${MIGRATION_INPUT}"
if [[ ! -e "$MIGRATION_FILE" ]]; then
  echo "::error::schemaguard-action: migration '$MIGRATION_INPUT' does not exist in the PR checkout (resolved to '$MIGRATION_FILE')"
  exit 1
fi

CONFIG_ARGS=()
if [[ -n "$CONFIG_INPUT" ]]; then
  CONFIG_FILE="${GITHUB_WORKSPACE}/${CONFIG_INPUT}"
  if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "::error::schemaguard-action: config '$CONFIG_INPUT' does not exist in the PR checkout (resolved to '$CONFIG_FILE')"
    exit 1
  fi
  CONFIG_ARGS+=("--config" "$CONFIG_FILE")
fi

# --- Invoke schemaguard check ----------------------------------------------

echo "==> Running schemaguard check --format markdown --out $REPORT_PATH"
# Note: `${CONFIG_ARGS[@]+"${CONFIG_ARGS[@]}"}` expands to the array
# elements when the array is non-empty and to nothing when it is
# empty, without tripping `set -u` on Bash 3.x (which is still the
# default on some runners).
"$SCHEMAGUARD_BIN" check \
  --migration "$MIGRATION_FILE" \
  --snapshot "$SNAPSHOT_FILE" \
  --format markdown \
  --out "$REPORT_PATH" \
  ${CONFIG_ARGS[@]+"${CONFIG_ARGS[@]}"}
SG_EXIT=$?
echo "==> schemaguard exited with code $SG_EXIT"

# --- Fail-closed logging (task 6.6) ----------------------------------------
# Print the report to the Action log unconditionally, BEFORE attempting
# any PR comment upsert. If the upsert fails (missing token, API
# outage, rate limit, permissions), findings are still visible in the
# workflow logs and can be copy-pasted into the PR manually.

if [[ -s "$REPORT_PATH" ]]; then
  echo "=============== SchemaGuard report ==============="
  cat "$REPORT_PATH"
  echo "=============== end SchemaGuard report ==============="
else
  echo "::warning::schemaguard-action: no report file was produced at $REPORT_PATH (exit code $SG_EXIT). Creating a minimal stand-in so a comment can still be posted."
  cat > "$REPORT_PATH" <<EOF
## 🛑 SchemaGuard tool error

SchemaGuard exited with code $SG_EXIT before producing a report. The
run could not complete. See the Action log above for details.
EOF
fi

# --- Map exit code to verdict + step outputs -------------------------------

case "$SG_EXIT" in
  0) VERDICT="green" ;;
  1) VERDICT="yellow" ;;
  2) VERDICT="red" ;;
  3) VERDICT="tool_error" ;;
  *) VERDICT="unknown" ;;
esac

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "verdict=$VERDICT"
    echo "exit-code=$SG_EXIT"
  } >> "$GITHUB_OUTPUT"
fi

# --- PR comment upsert (task 6.4) ------------------------------------------
# Only runs on pull_request / pull_request_target events. Any failure
# is downgraded to a warning because the report is already in the log.

UPSERT_ATTEMPTED=0
UPSERT_FAILED=0
EVENT_NAME="${GITHUB_EVENT_NAME:-}"
if [[ "$EVENT_NAME" == "pull_request" || "$EVENT_NAME" == "pull_request_target" ]]; then
  UPSERT_ATTEMPTED=1
  if [[ -z "${GITHUB_TOKEN:-}" ]]; then
    echo "::warning::schemaguard-action: GITHUB_TOKEN is unset; skipping PR comment upsert. Report is in the log above."
    UPSERT_FAILED=1
  else
    PR_NUMBER=""
    if [[ -f "${GITHUB_EVENT_PATH:-/dev/null}" ]]; then
      PR_NUMBER=$(jq -r '.pull_request.number // empty' "$GITHUB_EVENT_PATH" 2>/dev/null || echo "")
    fi
    if [[ -z "$PR_NUMBER" ]]; then
      echo "::warning::schemaguard-action: could not determine PR number from GITHUB_EVENT_PATH; skipping PR comment upsert."
      UPSERT_FAILED=1
    else
      if ! bash "${ACTION_DIR}/upsert-comment.sh" "${GITHUB_REPOSITORY}" "$PR_NUMBER" "$REPORT_PATH"; then
        echo "::warning::schemaguard-action: PR comment upsert failed. Report is still in the log above; continuing with exit code $SG_EXIT."
        UPSERT_FAILED=1
      fi
    fi
  fi
else
  echo "info: event '${EVENT_NAME:-unknown}' is not a pull_request; skipping PR comment upsert."
fi

# --- Exit-code mapping + step-status annotations (task 6.5) ---------------
# exit 0 → step success, green ✓ in Checks UI
# exit 1 (yellow) → step success with warning annotation, green ✓ with warning
# exit 2 (red) → step failure, red ✗, "Red verdict: do not merge" annotation
# exit 3 (tool error) → step failure, red ✗, distinct "tool error" annotation
# Red verdict (2) and tool error (3) are distinguishable via the
# annotation text, as required by tasks.md 6.5.

case "$SG_EXIT" in
  0)
    echo "::notice::schemaguard-action: green — migration is safe to merge."
    exit 0
    ;;
  1)
    echo "::warning::schemaguard-action: yellow — caution-level findings; review before merging."
    exit 0
    ;;
  2)
    echo "::error::schemaguard-action: RED verdict — stop-level findings or failed migration; do not merge. Review the PR comment or the report above for details."
    exit 1
    ;;
  3)
    echo "::error::schemaguard-action: SchemaGuard TOOL ERROR — the run could not complete (bad input, Docker unavailable, restore failure, or internal crash). This is distinct from a red verdict; fix the environment, not the migration."
    exit 1
    ;;
  *)
    echo "::error::schemaguard-action: unexpected exit code $SG_EXIT from schemaguard check"
    exit 1
    ;;
esac
