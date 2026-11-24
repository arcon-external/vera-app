# GTK/WebKit C toolchain for the `dev` and `production` Wails build tags.
#
# The #495 tag matrix compiles the root package with `-tags dev,webkit2_41`
# (and packaged/debug variants). `dev` and `production` select Wails' real
# Linux frontend, which cgo-links GTK 3 and WebKit; only the default tags use
# Wails' no-cgo stub. Ubuntu 24.04 packages WebKit 4.1, so Wails' `webkit2_41`
# tag is the one that makes pkg-config resolve `webkit2gtk-4.1`. Without the
# tag the build asks for `webkit2gtk-4.0`, which this distribution does not
# ship, and the failure is a pkg-config error before any Go file is compiled.
#
# No Go is installed on purpose. The consuming job mounts the GOROOT that
# actions/setup-go installed from go.mod -- read-only -- plus the module and
# build caches, and runs `go vet -tags dev,webkit2_41 ./ ./backend/auth` inside
# this image. The image holds the native dependencies only, so a Go bump can
# never publish a container that disagrees with go.mod.
#
# The base is pinned by digest. Dependabot's docker ecosystem watches
# `/.github/ci/gtk`, so the pin moves through a reviewed PR rather than
# floating; the weekly ci-image rebuild picks up Ubuntu package updates that do
# not need a new base digest.
FROM ubuntu:24.04@sha256:562111b88caa79c71391c0c4d3c2114a7d5c35915c20561fd893cb1e07929e6b

# The source label is what links the GHCR package to this repository, so the
# repository's GITHUB_TOKEN can publish it with `packages: write`.
LABEL org.opencontainers.image.source="https://github.com/arcon-external/vera-app" \
      org.opencontainers.image.title="Vera CI GTK/WebKit toolchain" \
      org.opencontainers.image.description="GTK 3, WebKitGTK 4.1, libsoup 3 and a C toolchain for the Vera Wails tag matrix; Go comes from the consuming job."

# One apt layer. The BuildKit cache mounts keep the downloaded .debs and package
# lists out of the image while letting a rebuild skip the mirror; the lists are
# removed afterwards so a build that cannot use the cache mount is not larger.
# sharing=locked stops two concurrent builds corrupting the same cache.
#
# --no-install-recommends matters here: the recommended packages pull a desktop,
# fonts and documentation that a compile-only image never reads.
ARG DEBIAN_FRONTEND=noninteractive
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
        pkg-config \
        build-essential \
        libgtk-3-dev \
        libwebkit2gtk-4.1-dev \
        libsoup-3.0-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
