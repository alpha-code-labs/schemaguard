#!/usr/bin/env bash
#
# One-command demo driver for Milestone 7.4.
#
# Builds the schemaguard CLI if missing, then runs `schemaguard check`
# against one (or all) of the four canonical failure migrations using
# the committed demo.dump fixture and schemaguard.yaml config.
#
# Usage:
#   bash demo/run-demo.sh               # run all four migrations
#   bash demo/run-demo.sh 01            # run migration 01 only
#   bash demo/run-demo.sh 01 02 03 04   # run any subset in order
#
# Prerequisites: Docker daemon running, Go toolchain installed. On
# the first run the Go build step compiles the CLI once and caches
# the binary at the repo root.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEMO_DIR="${ROOT}/demo"
SNAPSHOT="${DEMO_DIR}/snapshot/demo.dump"
CONFIG="${DEMO_DIR}/schemaguard.yaml"
BIN="${ROOT}/schemaguard"

# Associative arrays (`declare -A`) are a Bash 4 feature and macOS
# ships Bash 3.2 by default, so we resolve migration labels via a
# case statement for portability across both shells.

migration_for() {
  case "$1" in
    01) echo "${DEMO_DIR}/migrations/01_add_column_not_null_default.sql" ;;
    02) echo "${DEMO_DIR}/migrations/02_create_index_blocking.sql"      ;;
    03) echo "${DEMO_DIR}/migrations/03_rename_column.sql"              ;;
    04) echo "${DEMO_DIR}/migrations/04_add_check_constraint.sql"       ;;
    *)  echo ""                                                         ;;
  esac
}

LABELS="01 02 03 04"

if [[ ! -f "$SNAPSHOT" ]]; then
  echo "error: demo snapshot not found at $SNAPSHOT" >&2
  echo "       run 'bash demo/snapshot/build.sh' to regenerate it" >&2
  exit 1
fi
if [[ ! -f "$CONFIG" ]]; then
  echo "error: demo config not found at $CONFIG" >&2
  exit 1
fi

if [[ ! -x "$BIN" ]]; then
  echo "==> Building schemaguard CLI"
  (cd "$ROOT" && go build -o schemaguard ./cmd/schemaguard)
fi

run_one() {
  local label="$1"
  local file
  file="$(migration_for "$label")"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo "error: unknown or missing migration '$label'" >&2
    return 2
  fi
  echo
  echo "=============================================="
  echo "SchemaGuard demo: migration $label — $(basename "$file")"
  echo "=============================================="
  # Do not abort the driver on a non-zero exit — the whole point of
  # the demo is to show that these migrations are flagged, and each
  # flagged run exits with 1 (yellow) or 2 (red) by design.
  "$BIN" check \
    --migration "$file" \
    --snapshot "$SNAPSHOT" \
    --config "$CONFIG" || true
}

if [[ $# -eq 0 ]]; then
  for label in $LABELS; do
    run_one "$label"
  done
else
  for label in "$@"; do
    run_one "$label"
  done
fi
