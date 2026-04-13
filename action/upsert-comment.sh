#!/usr/bin/env bash
#
# SchemaGuard PR comment upsert helper.
#
# Given a repository, a PR number, and a Markdown report file, this
# script prepends a stable HTML-comment marker to the report body and
# either creates a new comment on the PR or updates the existing one
# carrying the marker. Exactly one comment per PR is maintained even
# across many commits or re-runs.
#
# Task 6.4 in docs/tasks.md:
#   "Post the Markdown report as a comment on the PR. On subsequent
#    runs, find the existing SchemaGuard comment and update it in
#    place rather than creating duplicates. Use a hidden marker (HTML
#    comment) for identification."
#
# The marker is intentionally versioned — if a future schema bump
# needs to invalidate old comments, we can change it and the upsert
# will create a fresh comment rather than mutating a stale one.
#
# Usage:
#   upsert-comment.sh <owner/repo> <pr-number> <report-file-path>
#
# Required environment: GITHUB_TOKEN, RUNNER_TEMP.

set -uo pipefail

REPO="${1:-}"
PR_NUMBER="${2:-}"
REPORT_FILE="${3:-}"

if [[ -z "$REPO" || -z "$PR_NUMBER" || -z "$REPORT_FILE" ]]; then
  echo "error: upsert-comment.sh: missing arguments" >&2
  echo "usage: upsert-comment.sh <owner/repo> <pr-number> <report-file>" >&2
  exit 2
fi

if [[ ! -f "$REPORT_FILE" ]]; then
  echo "error: upsert-comment.sh: report file not found at $REPORT_FILE" >&2
  exit 2
fi

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "error: upsert-comment.sh: GITHUB_TOKEN is unset" >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "error: upsert-comment.sh: jq is required but not installed on this runner" >&2
  exit 2
fi

MARKER="<!-- schemaguard-comment: v1 -->"

# Prepend the marker to the report body. The marker is an HTML
# comment so it is invisible in rendered Markdown but stable for
# grep-based lookup by subsequent runs.
BODY_FILE="${RUNNER_TEMP:-/tmp}/schemaguard-comment-body.md"
{
  printf '%s\n\n' "$MARKER"
  cat "$REPORT_FILE"
} > "$BODY_FILE"

API="https://api.github.com/repos/${REPO}"
AUTH_HEADER="Authorization: Bearer ${GITHUB_TOKEN}"
ACCEPT_HEADER="Accept: application/vnd.github+json"
UA_HEADER="User-Agent: schemaguard-action"
GH_API_VER="X-GitHub-Api-Version: 2022-11-28"

# --- Look up existing comment ---------------------------------------------
# Page through the issue comments looking for one whose body starts
# with the marker. GitHub pages at 30 by default; we ask for 100 to
# cut round-trips on busy PRs. We stop paging after 10 pages to avoid
# a pathological loop.

EXISTING_ID=""
PAGE=1
while [[ "$PAGE" -le 10 ]]; do
  LIST_URL="${API}/issues/${PR_NUMBER}/comments?per_page=100&page=${PAGE}"
  if ! RESP=$(curl --fail --silent --show-error \
      -H "$AUTH_HEADER" -H "$ACCEPT_HEADER" -H "$UA_HEADER" -H "$GH_API_VER" \
      "$LIST_URL"); then
    echo "error: upsert-comment.sh: failed to list comments on page $PAGE" >&2
    exit 1
  fi

  FOUND=$(echo "$RESP" | jq -r --arg marker "$MARKER" \
    '[.[] | select((.body // "") | startswith($marker)) | .id] | first // empty')
  if [[ -n "$FOUND" ]]; then
    EXISTING_ID="$FOUND"
    break
  fi

  COUNT=$(echo "$RESP" | jq 'length')
  if [[ "$COUNT" -lt 100 ]]; then
    break
  fi
  PAGE=$((PAGE + 1))
done

# Build the JSON payload. Using jq --rawfile is the only safe way to
# encode arbitrary Markdown (backticks, quotes, newlines, backslashes)
# into a JSON string without hand-escaping.
PAYLOAD=$(jq -n --rawfile body "$BODY_FILE" '{body: $body}')

if [[ -n "$EXISTING_ID" ]]; then
  echo "==> Updating existing SchemaGuard comment (id=$EXISTING_ID)"
  if ! curl --fail --silent --show-error \
      -X PATCH \
      -H "$AUTH_HEADER" -H "$ACCEPT_HEADER" -H "$UA_HEADER" -H "$GH_API_VER" \
      -H "Content-Type: application/json" \
      --data-binary "$PAYLOAD" \
      "${API}/issues/comments/${EXISTING_ID}" >/dev/null; then
    echo "error: upsert-comment.sh: failed to update existing comment $EXISTING_ID" >&2
    exit 1
  fi
else
  echo "==> Creating new SchemaGuard comment"
  if ! curl --fail --silent --show-error \
      -X POST \
      -H "$AUTH_HEADER" -H "$ACCEPT_HEADER" -H "$UA_HEADER" -H "$GH_API_VER" \
      -H "Content-Type: application/json" \
      --data-binary "$PAYLOAD" \
      "${API}/issues/${PR_NUMBER}/comments" >/dev/null; then
    echo "error: upsert-comment.sh: failed to create new comment" >&2
    exit 1
  fi
fi

echo "==> PR comment upserted successfully"
