#!/bin/bash
# Dependabot status: the open alerts and the open Dependabot pull requests.
#
# The release checklist (docs/agents/release.md) reads this before a tag, and
# every PR body carries the alert read (AGENTS.md §11). Doing it by hand is how
# six open Dependabot PRs got rediscovered mid-release: the queries are easy to
# half-remember, and an empty answer is indistinguishable from a query that
# failed unless the failure is loud.
#
# So this script:
#   - reads the alerts endpoint and the Dependabot-authored open PRs;
#   - merges every page of the alerts response into one list;
#   - prints `Dependabot: 0 open` when nothing is open, the literal AGENTS.md
#     §11 asks a PR body to carry, rather than printing nothing;
#   - fails with the API's own message when a read fails, never with an empty
#     list that looks like "nothing open".
#
# It is a status command: exit 0 means the read succeeded, not that the lists
# are empty. Whether an open item blocks the release is the checklist's call.
#
# Usage: scripts/dependabot-status.sh [--json] [--repo owner/name]
# Needs: gh (authenticated), jq
set -euo pipefail

REPO="${GITHUB_REPOSITORY:-arcon-external/vera-app}"
JSON=false

usage() {
  cat >&2 <<'EOF'
usage: dependabot-status.sh [--json] [--repo owner/name]

  --json   one JSON document on stdout, for a release check or a script:
           {repository, alerts: {count, items}, pullRequests: {count, items}}
  --repo   owner/name. Defaults to GITHUB_REPOSITORY, then
           arcon-external/vera-app.

exit status:
  0  the read succeeded; items may still be open
  1  a query failed; its message is on stderr
  2  usage error, or gh/jq is missing
EOF
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --json) JSON=true; shift ;;
    --repo)
      if [ $# -lt 2 ]; then
        echo "dependabot-status.sh: --repo needs owner/name" >&2
        usage
      fi
      REPO="$2"
      shift 2
      ;;
    -h|--help) usage ;;
    *)
      echo "dependabot-status.sh: unknown argument $1" >&2
      usage
      ;;
  esac
done

if ! [[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  echo "dependabot-status.sh: --repo '$REPO' is not owner/name" >&2
  exit 2
fi

for tool in gh jq; do
  if ! command -v "$tool" >/dev/null; then
    echo "dependabot-status.sh: $tool is required" >&2
    exit 2
  fi
done

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

# report_failure HEADLINE ERRFILE -- the headline, then whatever the tool said,
# indented. gh's own message is the useful part (a 403 names the permission it
# wanted); swallowing it would leave "cannot read alerts" with no next step.
report_failure() {
  printf 'dependabot-status: %s\n' "$1" >&2
  if [ -s "$2" ]; then
    sed 's/^/  /' "$2" >&2
  fi
}

# ---------------------------------------------------------------------------
# Alerts. --paginate prints each page as its own JSON array, so they are merged
# with `jq -s add`. A response that is not an array is a failure, not an empty
# list: the endpoint returns an object for some errors, and counting that as
# zero is the exact bug this script exists to avoid.
# ---------------------------------------------------------------------------
ALERTS_RAW="$WORKDIR/alerts.json"
ALERTS_ERR="$WORKDIR/alerts.err"
if ! gh api --paginate "repos/$REPO/dependabot/alerts?state=open&per_page=100" \
    >"$ALERTS_RAW" 2>"$ALERTS_ERR"; then
  report_failure "cannot read open alerts for $REPO" "$ALERTS_ERR"
  exit 1
fi
if ! ALERTS=$(jq -s -e 'add // [] | if type == "array" then . else error("not an array") end' \
    "$ALERTS_RAW" 2>"$WORKDIR/alerts-jq.err"); then
  report_failure "the alerts response for $REPO did not parse as a JSON array" "$WORKDIR/alerts-jq.err"
  exit 1
fi
if ! ALERTS_JSON=$(printf '%s' "$ALERTS" | jq -c '[.[] | {
      number: (.number // 0),
      severity: (.security_advisory.severity // "unknown"),
      package: (.dependency.package.name // "unknown"),
      firstPatched: (.security_vulnerability.first_patched_version.identifier // ""),
      url: (.html_url // "")
    }]'); then
  echo "dependabot-status: could not normalise the alerts response for $REPO" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Open Dependabot pull requests. app/dependabot is how gh names the bot; the
# JSON fields are exactly what the release issue needs to act on one.
# ---------------------------------------------------------------------------
PRS_RAW="$WORKDIR/prs.json"
PRS_ERR="$WORKDIR/prs.err"
if ! gh pr list --repo "$REPO" --author app/dependabot --state open --limit 100 \
    --json number,title,url,headRefName,createdAt \
    >"$PRS_RAW" 2>"$PRS_ERR"; then
  report_failure "cannot read open Dependabot pull requests for $REPO" "$PRS_ERR"
  exit 1
fi
if ! PRS_JSON=$(jq -c -e 'if type == "array" then
      [.[] | {number, title, url, headRefName, createdAt}]
    else error("not an array") end' "$PRS_RAW" 2>"$WORKDIR/prs-jq.err"); then
  report_failure "the pull request response for $REPO did not parse as a JSON array" "$WORKDIR/prs-jq.err"
  exit 1
fi

if [ "$JSON" = true ]; then
  jq -n --arg repo "$REPO" --argjson alerts "$ALERTS_JSON" --argjson prs "$PRS_JSON" '{
    repository: $repo,
    alerts: {count: ($alerts | length), items: $alerts},
    pullRequests: {count: ($prs | length), items: $prs}
  }'
  exit 0
fi

ALERT_COUNT=$(printf '%s' "$ALERTS_JSON" | jq 'length')
PR_COUNT=$(printf '%s' "$PRS_JSON" | jq 'length')

printf 'Dependabot: %s open\n' "$ALERT_COUNT"
if [ "$ALERT_COUNT" -gt 0 ]; then
  printf '%s' "$ALERTS_JSON" | jq -r '.[] |
    "  \(.severity)\t\(.package)\t\(if .firstPatched == "" then "no patch published" else .firstPatched end)\t\(.url)"'
fi

printf '\n'
printf 'Dependabot pull requests: %s open\n' "$PR_COUNT"
if [ "$PR_COUNT" -gt 0 ]; then
  printf '%s' "$PRS_JSON" | jq -r '.[] | "  #\(.number)\t\(.title)\t\(.url)"'
fi
