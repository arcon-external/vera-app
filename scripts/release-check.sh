#!/bin/bash
# Release check: does the changelog name every fix/feat commit in a tag range?
#
# The release checklist (docs/agents/release.md) writes the changelog before the
# tag. The What's New modal ships that section, so a fix that shipped without a
# note is a fix staff never hear about. Reading the commits by hand is how that
# happens: a merged branch is easy to forget, and a subject is easy to skim.
#
# So this script:
#   - reads the fix/feat subjects in the range with `git log --format=%s`;
#   - extracts each commit's issue references (#NNN) and, when it has none, uses
#     its subject as the identifier;
#   - matches those against the release section of the changelog: the section
#     named by the range's end tag, or the first section when the end is not
#     tagged (the release being prepared);
#   - fails when a fix/feat commit is not named, printing each missing
#     reference (or subject), and passes when every one is.
#
# It never writes: --check and --dry-run are the same read, accepted so the
# checklist and announce-release.sh's vocabulary agree.
#
# Usage: scripts/release-check.sh [--range REV..REV] [--changelog PATH]
#                                 [--json] [--check|--dry-run]
# Needs: git; jq only for --json
set -euo pipefail

CHANGELOG="CHANGELOG.md"
RANGE=""
JSON=false

usage() {
  cat >&2 <<'EOF'
usage: release-check.sh [--range REV..REV] [--changelog PATH] [--json] [--check|--dry-run]

  --range      the git range to read, e.g. v0.15.1..v0.15.2 or v0.15.1..HEAD.
               Defaults to the last two reachable tags.
  --changelog  the changelog file. Defaults to CHANGELOG.md.
  --json       one JSON document on stdout:
               {range, changelog, section, total, matched, missing, entries}
  --check      report only; this script never writes. Accepted for the checklist.
  --dry-run    alias for --check, matching announce-release.sh's vocabulary.

The check reads only the release section: the section named by the range's end
tag, or the first section when the end is not tagged (the release being prepared).

exit status:
  0  every fix/feat commit is named in the release section
  1  one or more fix/feat commits are not named
  2  usage error, missing file, or an unusable git range
EOF
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --range)
      if [ $# -lt 2 ]; then
        echo "release-check.sh: --range needs a revision range" >&2
        usage
      fi
      RANGE="$2"
      shift 2
      ;;
    --changelog)
      if [ $# -lt 2 ]; then
        echo "release-check.sh: --changelog needs a path" >&2
        usage
      fi
      CHANGELOG="$2"
      shift 2
      ;;
    --json) JSON=true; shift ;;
    --check|--dry-run) shift ;;
    -h|--help) usage ;;
    *)
      echo "release-check.sh: unknown argument $1" >&2
      usage
      ;;
  esac
done

if ! command -v git >/dev/null; then
  echo "release-check.sh: git is required" >&2
  exit 2
fi
if [ "$JSON" = true ] && ! command -v jq >/dev/null; then
  echo "release-check.sh: jq is required for --json" >&2
  exit 2
fi
if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "release-check.sh: not inside a git repository" >&2
  exit 2
fi
if [ ! -f "$CHANGELOG" ]; then
  echo "release-check.sh: changelog '$CHANGELOG' not found" >&2
  exit 2
fi
if [ ! -r "$CHANGELOG" ]; then
  echo "release-check.sh: changelog '$CHANGELOG' is not readable" >&2
  exit 2
fi

if [ -z "$RANGE" ]; then
  latest=$(git describe --tags --abbrev=0 2>/dev/null) || {
    echo "release-check.sh: no reachable tag; pass --range" >&2
    exit 2
  }
  previous=$(git describe --tags --abbrev=0 "${latest}^" 2>/dev/null) || {
    echo "release-check.sh: $latest is the only reachable tag; pass --range" >&2
    exit 2
  }
  RANGE="${previous}..${latest}"
fi

if ! git rev-list --count "$RANGE" >/dev/null 2>&1; then
  echo "release-check.sh: '$RANGE' is not a valid git revision range" >&2
  exit 2
fi

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT
SECTION_FILE="$WORKDIR/section.md"
ENTRIES_JSONL="$WORKDIR/entries.jsonl"
: > "$ENTRIES_JSONL"

# first_section prints the first `## [name]` heading's name, without a leading
# `v`, so the pending release can be selected before its tag exists.
first_section() {
  awk '
    /^## \[/ {
      line = $0
      sub(/^## \[/, "", line)
      sub(/\].*$/, "", line)
      sub(/^v/, "", line)
      print line
      exit
    }
  ' "$CHANGELOG"
}

# extract_section writes the body of `## [VERSION]` (or `## [vVERSION]`) to
# SECTION_FILE and exits 3 when the changelog has no such section.
extract_section() {
  awk -v ver="$1" '
    /^## \[/ {
      if (found) exit 0
      line = $0
      sub(/^## \[/, "", line)
      sub(/\].*$/, "", line)
      sub(/^v/, "", line)
      if (line == ver) { found = 1 }
      next
    }
    found { print }
    END { if (!found) exit 3 }
  ' "$CHANGELOG" > "$SECTION_FILE"
}

END_REV="${RANGE##*..}"
if end_tag=$(git describe --tags --exact-match "$END_REV" 2>/dev/null); then
  SECTION="${end_tag#v}"
  if ! extract_section "$SECTION"; then
    echo "release-check.sh: $CHANGELOG has no '## [$SECTION]' section for $end_tag" >&2
    exit 2
  fi
else
  SECTION=$(first_section)
  if [ -z "$SECTION" ]; then
    echo "release-check.sh: $CHANGELOG has no '## [version]' heading" >&2
    exit 2
  fi
  if ! extract_section "$SECTION"; then
    echo "release-check.sh: could not read the '$SECTION' section of $CHANGELOG" >&2
    exit 2
  fi
fi

total=0
matched_count=0
missing_count=0
missing_lines=()

while IFS= read -r subject; do
  # Only fix/feat subjects are release notes; chore/docs/test/merge are not.
  if ! [[ "$subject" =~ ^(fix|feat)(\([^\)]*\))?!?:[[:space:]]+(.*)$ ]]; then
    continue
  fi
  desc="${BASH_REMATCH[3]}"
  total=$((total + 1))

  refs=()
  while IFS= read -r ref; do
    [ -n "$ref" ] && refs+=("$ref")
  done < <(grep -oE '#[0-9]+' <<<"$subject" | sort -u || true)

  matched=false
  for ref in "${refs[@]}"; do
    # Match the reference as a token: #562 must not be satisfied by #5620.
    num="${ref#\#}"
    if grep -Eq "#${num}([^0-9]|$)" "$SECTION_FILE"; then
      matched=true
      break
    fi
  done
  # A commit with no issue reference is identified by its subject. The
  # comparison ignores case, whitespace runs and markdown emphasis, so
  # `fix: repair the export button` matches `**Repair the export button.**`.
  if [ "$matched" = false ]; then
    needle=$(printf '%s' "$desc" | tr '[:upper:]' '[:lower:]' | tr -d '*' | tr -s '[:space:]' ' ')
    needle="${needle# }"
    needle="${needle% }"
    haystack=$(tr '[:upper:]' '[:lower:]' < "$SECTION_FILE" | tr -d '*' | tr -s '[:space:]' ' ')
    if [ -n "$needle" ] && [[ "$haystack" == *"$needle"* ]]; then
      matched=true
    fi
  fi

  if [ "$matched" = true ]; then
    matched_count=$((matched_count + 1))
  else
    missing_count=$((missing_count + 1))
    if [ ${#refs[@]} -gt 0 ]; then
      missing_lines+=("  ${refs[*]}  $subject")
    else
      missing_lines+=("  $subject")
    fi
  fi

  if [ "$JSON" = true ]; then
    if [ ${#refs[@]} -eq 0 ]; then
      refs_json="[]"
    else
      refs_json=$(printf '%s\n' "${refs[@]}" | jq -R . | jq -s -c .)
    fi
    jq -cn --arg subject "$subject" --argjson references "$refs_json" --argjson matched "$matched" \
      '{subject: $subject, references: $references, matched: $matched}' >> "$ENTRIES_JSONL"
  fi
done < <(git log --format=%s "$RANGE")

if [ "$JSON" = true ]; then
  jq -n \
    --arg range "$RANGE" \
    --arg changelog "$CHANGELOG" \
    --arg section "$SECTION" \
    --argjson total "$total" \
    --argjson matched "$matched_count" \
    --argjson missing "$missing_count" \
    --slurpfile entries "$ENTRIES_JSONL" \
    '{range: $range, changelog: $changelog, section: $section, total: $total, matched: $matched, missing: $missing, entries: $entries}'
else
  echo "release-check: $RANGE (section $SECTION) against $CHANGELOG"
  echo "  fix/feat commits: $total"
  echo "  matched: $matched_count"
  echo "  missing: $missing_count"
fi

if [ "$missing_count" -gt 0 ]; then
  if [ "$JSON" = false ]; then
    echo
    echo "missing from the changelog:"
    printf '%s\n' "${missing_lines[@]}"
  fi
  echo "release-check: $missing_count fix/feat commit(s) missing from $CHANGELOG (section $SECTION)" >&2
  exit 1
fi

if [ "$JSON" = false ]; then
  echo
  echo "release-check: OK — every fix/feat commit is named in the changelog"
fi
