#!/bin/bash
# Announce a release: mirror its artifacts (desktop installers and the CLI) to
# desktop updater at it.
#
# The updater learns about releases from six VERA_APP_* variables on the vera-api
# Cloud Run service, and reads them through GET /v1/app/latest. Nothing else
# announces anything -- not the GitHub Release, not the tag. So a release that
# does not set them is a release nobody is offered, which is exactly what
# happened to v0.4.0: it shipped, the installers were on the bucket, and the app
# went on offering v0.3.5 until a user said so.
#
# This script is that step, and it is deliberately ordered so the app is only
# told about a release that is already downloadable:
#
#   1. guard against a downgrade           -- fail before touching anything
#   2. check the release has all 3 assets  -- no announcement without files
#   3. download and verify checksums       -- the mirror is of known bytes
#   4. mirror to the bucket, then verify   -- public URLs must serve before 5
#   5. set the six variables               -- preserve min_supported from live
#   6. verify /v1/app/latest               -- fail if the app still cannot see it
#
# Step 4 before step 5 is the point of the whole ordering. Setting the variables
# first would tell every installed desktop to download URLs that 404, which is a
# worse outcome than telling it nothing.
#
# Usage:  scripts/announce-release.sh --version v0.4.0 [--dry-run]
# Needs:  gh (release assets), gcloud (bucket + service), curl, jq
set -euo pipefail

VERSION=""
DRY_RUN=false
PROJECT="${ANNOUNCE_PROJECT:-vera-app-508411}"
REGION="${ANNOUNCE_REGION:-asia-southeast1}"
SERVICE="${ANNOUNCE_SERVICE:-vera-api}"
BUCKET="${ANNOUNCE_BUCKET:-vera-downloads-508411}"
PUBLIC_API="${ANNOUNCE_API:-https://api.arconpentapersada.co.id}"

usage() {
  cat >&2 <<'EOF'
usage: announce-release.sh --version vX.Y.Z [--dry-run]

  --version   the release tag, e.g. v0.4.0. Used verbatim in asset names and
              as VERA_APP_LATEST, because that is how the release names them.
  --dry-run   run every check, mutate nothing. Reports what each step would do.

environment overrides (all optional):
  ANNOUNCE_PROJECT  ANNOUNCE_REGION  ANNOUNCE_SERVICE  ANNOUNCE_BUCKET
  ANNOUNCE_API     GITHUB_REPOSITORY
EOF
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --dry-run) DRY_RUN=true; shift ;;
    -h|--help) usage ;;
    *) echo "announce-release.sh: unknown argument $1" >&2; usage ;;
  esac
done

if [ -z "$VERSION" ]; then
  usage
fi
if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "announce-release.sh: --version '$VERSION' is not vMAJOR.MINOR.PATCH" >&2
  exit 2
fi
if ! command -v jq >/dev/null; then
  echo "announce-release.sh: jq is required" >&2
  exit 2
fi

REPO="${GITHUB_REPOSITORY:-arcon-external/vera-app}"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

# The three names are a contract with the updater, written by desktop.yml from
# the tag. Never assembled differently here -- if these ever change, they change
# in both places or the app links 404.
# The desktop installers, and the CLI beside them.
#
# The CLI is in this list on purpose: staff download from the bucket mirror,
# never from GitHub release assets, so a binary that is not mirrored is a binary
# nobody can download. Listing it here means a release whose CLI failed to build
# stops the announcement rather than shipping a documented download link that
# 404s.
ASSETS=(
  "Vera-${VERSION}-windows-amd64-installer.exe"
  "Vera-${VERSION}-windows-arm64-installer.exe"
  "Vera-${VERSION}-macos-universal.dmg"
  "Vera-CLI-${VERSION}-linux-amd64"
  "Vera-CLI-${VERSION}-linux-arm64"
  "Vera-CLI-${VERSION}-darwin-amd64"
  "Vera-CLI-${VERSION}-darwin-arm64"
  "Vera-CLI-${VERSION}-windows-amd64.exe"
  "Vera-CLI-${VERSION}-windows-arm64.exe"
)
BUCKET_BASE="https://storage.googleapis.com/${BUCKET}"

step()  { printf '\n== %s\n' "$*"; }
fail()  { printf 'announce-release: %s\n' "$*" >&2; exit 1; }
note()  { printf '   %s\n' "$*"; }

# semver_gt A B -- true when A sorts after B. sort -V is enough for the
# vMAJOR.MINOR.PATCH tags this accepts; a pre-release suffix sorts below its
# release, which is the behaviour we want.
semver_gt() { [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1)" = "$1" ] && [ "$1" != "$2" ]; }

# public_latest -- the updater's own view of the world, straight from the API the
# app calls. Retried because Cloud Run scales to zero here and the first request
# after a cold start can take a while.
public_latest() {
  local body="" i
  for i in $(seq 1 10); do
    if body=$(curl -sf -m 15 "$PUBLIC_API/v1/app/latest" 2>/dev/null) && [ -n "$body" ]; then
      printf '%s' "$body"
      return 0
    fi
    note "updater endpoint not answering yet (attempt $i/10)"
    sleep 5
  done
  return 1
}

# http_code URL -- the status only. Range requests are used so a 100 MB probe
# does not download 100 MB.
http_code() {
  curl -s -o /dev/null -w '%{http_code}' -m 30 -r 0-1 "$1" 2>/dev/null || echo "000"
}

sha_check() {
  local file="$1" sumfile="$2" want got
  want=$(awk '{print $1}' "$sumfile")
  if command -v sha256sum >/dev/null; then
    got=$(sha256sum "$file" | awk '{print $1}')
  else
    got=$(shasum -a 256 "$file" | awk '{print $1}')
  fi
  [ "$want" = "$got" ] || fail "$(basename "$file") checksum mismatch: expected $want, got $got"
}

printf 'announce %s%s\n' "$VERSION" "$([ "$DRY_RUN" = true ] && printf ' (dry run, nothing will be changed)')"
printf '   repo %s  service %s  bucket %s\n' "$REPO" "$SERVICE" "$BUCKET"

# ---------------------------------------------------------------------------
step "1/6  guard against announcing a downgrade"
# Tagging an old version after a newer one would roll the announcement back and
# tell every installed desktop to downgrade. That is never what a release means,
# and an intentional rollback is rare enough to do by hand.
# ---------------------------------------------------------------------------
LIVE_BODY=$(public_latest) || fail "cannot read $PUBLIC_API/v1/app/latest; refusing to touch the service blind"
LIVE_LATEST=$(printf '%s' "$LIVE_BODY" | jq -r '.latest // ""')
LIVE_MIN=$(printf '%s' "$LIVE_BODY" | jq -r '.minSupported // ""')

if [ -n "$LIVE_LATEST" ] && semver_gt "$LIVE_LATEST" "$VERSION"; then
  fail "live VERA_APP_LATEST is $LIVE_LATEST, which is newer than $VERSION.
   Announcing $VERSION would tell every installed copy to downgrade.
   For an intentional rollback, set the variables directly:
     gcloud run services update $SERVICE --project=$PROJECT --region=$REGION \\
       --update-env-vars \"VERA_APP_LATEST=$VERSION,...\""
fi
if [ "$LIVE_LATEST" = "$VERSION" ]; then
  note "live already announces $VERSION; this is a re-announce and will converge"
elif [ -n "$LIVE_LATEST" ]; then
  note "live announces $LIVE_LATEST, moving to $VERSION"
fi

# min_supported is a deliberate, occasional decision by a human. Read it live and
# carry it forward; hard-coding a default here would quietly revert every
# deliberate bump on the next release.
if [ -z "$LIVE_MIN" ]; then
  fail "live service reports no minSupported; refusing to guess a floor"
fi
note "min_supported carried forward from live: $LIVE_MIN"

# ---------------------------------------------------------------------------
step "2/6  check the release carries every artifact"
# The download URLs below point at these files. Announcing a release whose
# assets are still building would hand staff a 404 on first click.
# ---------------------------------------------------------------------------
if ! command -v gh >/dev/null; then
  fail "gh is required to read the release"
fi
RELEASE_JSON=$(gh release view "$VERSION" --repo "$REPO" --json isDraft,assets 2>/dev/null) \
  || fail "release $VERSION not found in $REPO. Create the release before tagging -- desktop.yml
   uploads with 'gh release upload', which fails with 'release not found' otherwise."
[ "$(printf '%s' "$RELEASE_JSON" | jq -r '.isDraft')" = "false" ] \
  || fail "release $VERSION is still a draft; the app must never be pointed at one"

for asset in "${ASSETS[@]}"; do
  size=$(printf '%s' "$RELEASE_JSON" | jq -r --arg n "$asset" '.assets[]? | select(.name == $n) | .size // empty')
  [ -n "$size" ] || fail "release $VERSION has no asset named $asset.
   The announce step must not advertise a file that does not exist."
  note "$(printf '%-46s %10s bytes' "$asset" "$size")"
done

# ---------------------------------------------------------------------------
step "3/6  download and verify checksums"
# desktop.yml writes a .sha256 beside each asset. Verifying them here means the
# bucket only ever receives bytes the release vouched for.
# ---------------------------------------------------------------------------
for asset in "${ASSETS[@]}"; do
  gh release download "$VERSION" --repo "$REPO" --pattern "$asset" --pattern "$asset.sha256" \
    --dir "$WORKDIR" --clobber >/dev/null 2>&1 \
    || fail "could not download $asset from release $VERSION"
  sha_check "$WORKDIR/$asset" "$WORKDIR/$asset.sha256"
  note "$(printf '%-46s checksum OK' "$asset")"
done

# ---------------------------------------------------------------------------
step "4/6  mirror to gs://${BUCKET} and prove the public URLs serve"
# The bucket is the only download route the app is allowed to hand out: every
# URL below is proven to serve before anything is announced, and the bucket
# does not depend on the repository's visibility.
# ---------------------------------------------------------------------------
if [ "$DRY_RUN" = true ]; then
  note "dry run: would upload ${#ASSETS[@]} files to gs://${BUCKET}/"
else
  command -v gcloud >/dev/null || fail "gcloud is required to write the bucket"
  # The .sha256 sidecars go up too. They were downloaded in step 3 purely to
  # verify the installers, but the release notes point staff at them as the way
  # to check a download, and a note that links a 404 is worse than no note.
  for asset in "${ASSETS[@]}" "${ASSETS[@]/%/.sha256}"; do
    gcloud storage cp "$WORKDIR/$asset" "gs://${BUCKET}/${asset}" \
      --project="$PROJECT" --quiet >/dev/null 2>&1 \
      || fail "could not upload $asset to gs://${BUCKET}/; VERA_APP_LATEST left at $LIVE_LATEST.
   Most likely the workflow's service account (github-deployer@) lacks
   roles/storage.objectAdmin on gs://${BUCKET}/ -- one grant, see
   docs/api-deploy.md. It can also be mirrored by hand from a machine
   with bucket write access, then this job re-run with --failed."
  done
  note "uploaded $(( ${#ASSETS[@]} * 2 )) files (installers and checksums)"
fi

for asset in "${ASSETS[@]}" "${ASSETS[@]/%/.sha256}"; do
  remote=$(gcloud storage ls "gs://${BUCKET}/${asset}" --project="$PROJECT" 2>/dev/null | grep -c . || true)
  [ "$remote" -ge 1 ] || fail "gs://${BUCKET}/${asset} is missing after upload"
  code=$(http_code "${BUCKET_BASE}/${asset}")
  case "$code" in
    200|206) note "$(printf '%-46s HTTP %s' "$asset" "$code")" ;;
    *)       fail "${BUCKET_BASE}/${asset} returned HTTP ${code}; staff would get a broken download link" ;;
  esac
done

# ---------------------------------------------------------------------------
step "5/6  set the six updater variables on ${SERVICE}"
# ---------------------------------------------------------------------------
ENV_VARS="VERA_APP_LATEST=${VERSION}"
ENV_VARS="${ENV_VARS},VERA_APP_MIN_SUPPORTED=${LIVE_MIN}"
ENV_VARS="${ENV_VARS},VERA_APP_NOTES_URL=https://github.com/${REPO}/releases/tag/${VERSION}"
ENV_VARS="${ENV_VARS},VERA_APP_DOWNLOAD_WINDOWS=${BUCKET_BASE}/Vera-${VERSION}-windows-amd64-installer.exe"
ENV_VARS="${ENV_VARS},VERA_APP_DOWNLOAD_WINDOWS_ARM64=${BUCKET_BASE}/Vera-${VERSION}-windows-arm64-installer.exe"
ENV_VARS="${ENV_VARS},VERA_APP_DOWNLOAD_MACOS=${BUCKET_BASE}/Vera-${VERSION}-macos-universal.dmg"

if [ "$DRY_RUN" = true ]; then
  note "dry run: would run"
  printf '     gcloud run services update %s --project=%s --region=%s \\\n' "$SERVICE" "$PROJECT" "$REGION"
  printf '       --update-env-vars "%s"\n' "$ENV_VARS"
else
  gcloud run services update "$SERVICE" --project="$PROJECT" --region="$REGION" \
    --update-env-vars "$ENV_VARS" --quiet >/dev/null 2>&1 \
    || fail "could not update $SERVICE; the release is on GitHub and the bucket but the app will not be told"
  revision=$(gcloud run services describe "$SERVICE" --project="$PROJECT" \
    --region="$REGION" --format='value(status.latestReadyRevisionName)' 2>/dev/null || echo unknown)
  note "updated $SERVICE, now serving $revision"
fi

# ---------------------------------------------------------------------------
step "6/6  verify the app can actually see it"
# This is the check whose absence made v0.4.0 invisible. It is the last line of
# defence: if the running service still reports the old version, the job fails
# here rather than reporting a release that nobody will be offered.
# ---------------------------------------------------------------------------
if [ "$DRY_RUN" = true ]; then
  note "dry run: skipped (the service was not touched, so ${VERSION} cannot be live)"
  step "dry run complete -- every check above passed, nothing was changed"
  exit 0
fi

ANNOUNCED=""
for i in $(seq 1 15); do
  sleep 5
  ANNOUNCED=$(public_latest) || continue
  got=$(printf '%s' "$ANNOUNCED" | jq -r '.latest // ""')
  if [ "$got" = "$VERSION" ]; then
    break
  fi
  note "still reports $got (attempt $i/15)"
done

if [ -z "$ANNOUNCED" ]; then
  fail "$PUBLIC_API/v1/app/latest returned nothing after repeated attempts; the service is up but not answering"
fi
# Not written as "${ANNOUNCED:-{}}": bash closes that expansion at the first
# brace and appends a literal } to a successful response, which jq then
# rejects -- a green announcement false-failing on its own verification.
got=$(printf '%s' "$ANNOUNCED" | jq -r '.latest // ""')
[ "$got" = "$VERSION" ] \
  || fail "$PUBLIC_API/v1/app/latest reports latest=$got, want $VERSION.
   The release exists and the files are on the bucket, but no installed copy
   will be told about it. This is the v0.4.0 failure mode."

printf '   %-16s %s\n' "latest" "$got"
printf '   %-16s %s\n' "minSupported" "$(printf '%s' "$ANNOUNCED" | jq -r '.minSupported')"

for key in windows windows-arm64 macos; do
  url=$(printf '%s' "$ANNOUNCED" | jq -r ".downloads[\"$key\"] // empty")
  [ -n "$url" ] || fail "updater response has no downloads.$key; the app would render a dead button"
  case "$url" in
    *"$BUCKET_BASE"/*"$VERSION"*) : ;;
    *) fail "downloads.$key points at $url, which is neither the public bucket nor $VERSION.
       GitHub release assets are not the staff download route: only the public
       bucket is proven to serve before anything is announced." ;;
  esac
  code=$(http_code "$url")
  case "$code" in
    200|206) printf '   %-16s HTTP %s  %s\n' "$key" "$code" "${url##*/}" ;;
    *)       fail "the updater's own downloads.$key returned HTTP $code" ;;
  esac
done

step "announced ${VERSION}"
printf '   installed copies will be offered %s on their next update check\n' "$VERSION"
