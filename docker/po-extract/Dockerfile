# Issue #477: the text-PDF extractor image.
#
# The image is deliberately small and one-purpose: a static Go binary and the
# Poppler command it drives. It contains no application secrets, no database
# client, no shell entrypoint and no network client beyond what Debian's base
# provides. The server runs it with --network none.
#
# This is the only supported production configuration for extraction. The
# server image (Dockerfile.veraserver) ships neither this binary nor Poppler,
# because a same-UID subprocess in the server container can read the parent's
# /proc/<pid>/environ and recover its credentials.
#
# Both base images are pinned by digest and poppler-utils is pinned to an exact
# Debian package version. The installed Poppler version is recorded at build
# time in /etc/vextract-engine-version and is also reported by
# `vextract -version`; the extraction result carries it, so a repeat run is
# comparable and a regression is attributable to a version. The license review
# basis and source-offer note live in the RUN comment below.
FROM golang:1.26-bookworm@sha256:d9c68c2c51161e12fd77e4c6320687c9cd86e1af1e3ad6e6cd63ff970641453c AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/vextract ./cmd/vextract

FROM debian:trixie-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad

# poppler-utils is the only added package, pinned to the exact Debian package
# version this configuration was built and reviewed against. The version is
# recorded here rather than hard-coded in Go, so rebuilding with a different
# Poppler cannot leave the recorded version lying: if the suite rotates the
# expected package away, the build fails loudly instead of drifting.
#
# LICENSE REVIEW BASIS (#477, user-approved 2026-10-21): Poppler is GPL. Vera
# uses it as a separate server-side subprocess inside this dedicated image and
# does not link it into the desktop application or the server binary. The image
# is the reviewed configuration. Corresponding source for the exact Debian
# package is obtainable from the Debian archive for trixie
# (`apt-get source poppler=25.03.0-5+deb13u4`) and from the matching Poppler
# 25.03.0 release; distributing a rebuilt image carries the usual source-offer
# duty for that GPL program, which the release notes must satisfy. The server
# image (Dockerfile.veraserver) ships no Poppler and no extractor.
RUN apt-get update \
    && apt-get install -y --no-install-recommends poppler-utils=25.03.0-5+deb13u4 \
    && rm -rf /var/lib/apt/lists/* \
    && pdftotext -v 2>&1 | head -n 1 > /etc/vextract-engine-version \
    && chmod 0444 /etc/vextract-engine-version

COPY --from=build /out/vextract /usr/local/bin/vextract

# Numeric unprivileged user; no home directory or shell is needed. The server
# also runs the container read-only with a small /tmp tmpfs.
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/vextract"]
