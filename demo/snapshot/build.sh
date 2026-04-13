#!/usr/bin/env bash
#
# Regenerates demo/snapshot/demo.dump from demo/snapshot/seed.sql.
#
# Spins up a temporary postgres:16-alpine container, runs the seed
# SQL, and `pg_dump -Fc`s the result into the demo.dump fixture that
# the demo CI and the local run-demo.sh script consume. The dump uses
# Postgres custom format so `pg_restore` (which the SchemaGuard
# shadow-DB runner dispatches on the .dump extension) can load it
# back fast.
#
# Usage:
#   bash demo/snapshot/build.sh
#
# Prerequisites: Docker, a network connection to pull postgres:16-
# alpine the first time.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SEED_SQL="${SCRIPT_DIR}/seed.sql"
OUT_DUMP="${SCRIPT_DIR}/demo.dump"
CONTAINER="schemaguard-demo-seed-$$"
IMAGE="postgres:16-alpine"

cleanup() {
  docker stop "$CONTAINER" >/dev/null 2>&1 || true
  docker rm   "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [[ ! -f "$SEED_SQL" ]]; then
  echo "error: seed SQL not found at $SEED_SQL" >&2
  exit 1
fi

echo "==> Starting temporary Postgres ($IMAGE) as $CONTAINER"
docker run -d --rm \
  --name "$CONTAINER" \
  -e POSTGRES_HOST_AUTH_METHOD=trust \
  "$IMAGE" >/dev/null

echo "==> Waiting for Postgres readiness"
for i in $(seq 1 120); do
  if docker exec "$CONTAINER" pg_isready -U postgres -h 127.0.0.1 >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
if ! docker exec "$CONTAINER" pg_isready -U postgres -h 127.0.0.1 >/dev/null 2>&1; then
  echo "error: Postgres failed to become ready inside $CONTAINER" >&2
  exit 1
fi

echo "==> Running seed SQL (this can take a minute for ~1M rows)"
docker exec -i "$CONTAINER" \
  psql -U postgres -d postgres -v ON_ERROR_STOP=1 -X -q < "$SEED_SQL"

echo "==> Dumping to $OUT_DUMP (pg_dump -Fc, compression level 5)"
docker exec "$CONTAINER" \
  pg_dump -U postgres -d postgres -Fc -Z 5 > "$OUT_DUMP"

SIZE=$(du -h "$OUT_DUMP" | cut -f1)
echo "==> Dump size: $SIZE"
echo "==> Done"
