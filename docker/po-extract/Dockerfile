# Issue #477/#478: the text + OCR PDF extractor image.
#
# The image is deliberately small and one-purpose: a static Go binary, the
# Poppler commands it drives (pdftotext, pdftoppm) and Tesseract. It contains
# no application secrets, no database client, no shell entrypoint and no
# network client beyond what Debian's base provides. The server runs it with
# --network none.
#
# This is the only supported production configuration for extraction. The
# server image (Dockerfile.veraserver) ships neither this binary, Poppler nor
# Tesseract, because a same-UID subprocess in the server container can read the
# parent's /proc/<pid>/environ and recover its credentials.
#
# Both base images are pinned by digest and every engine package is pinned to an
# exact Debian package version. The builder pin is the multi-arch index digest
# (amd64 and arm64 resolve their own manifest); the runtime pin is a manifest
# digest. The installed Poppler and Tesseract versions are
# recorded at build time in /etc/vextract-engine-version and
# /etc/vextract-ocr-version, and every extraction result carries them, so a
# repeat run is comparable and a regression is attributable to a version. The
# license review basis and source-offer notes live in the RUN comment below.
FROM golang:1.26.10-bookworm@sha256:632dff7351799a5a9954a48dc06670ade72ee44e8c351cdc0e5a57af2fea21af AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/vextract ./cmd/vextract

FROM debian:trixie-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad

# poppler-utils and tesseract-ocr are pinned to the exact Debian package
# versions this configuration was built and reviewed against. The versions are
# recorded here rather than hard-coded in Go, so rebuilding with a different
# engine cannot leave the recorded version lying: if the suite rotates the
# expected package away, the build fails loudly instead of drifting.
#
# Tesseract uses eng and ind only. Those are the languages the documents
# actually use; adding a language is a reviewed change because it changes both
# the image size and the recognition result.
#
# LICENSE REVIEW BASIS (#478, user-approved 2026-10-21): Tesseract itself is
# Apache-2.0, and the Debian tesseract-ocr-eng / tesseract-ocr-ind traineddata
# are distributed under Apache-2.0 by the Tesseract tessdata project. Vera uses
# them as separate server-side subprocesses inside this dedicated image and
# does not link them into the desktop application or the server binary. The
# image is the reviewed configuration; the Debian package copyright files
# (/usr/share/doc/tesseract-ocr*/copyright) are the attribution record, and
# distributing a rebuilt image carries the usual notice duty for Apache-2.0
# material. Poppler remains GPL: corresponding source for the exact Debian
# package is obtainable from the Debian archive for trixie
# (`apt-get source poppler=25.03.0-5+deb13u4`) and from the matching Poppler
# 25.03.0 release. The server image (Dockerfile.veraserver) ships no Poppler, no
# Tesseract and no extractor.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        poppler-utils=25.03.0-5+deb13u4 \
        tesseract-ocr=5.5.0-1+b1 \
        tesseract-ocr-eng=1:4.1.0-2 \
        tesseract-ocr-ind=1:4.1.0-2 \
    && rm -rf /var/lib/apt/lists/* \
    && pdftotext -v 2>&1 | head -n 1 > /etc/vextract-engine-version \
    && tesseract --version 2>&1 | head -n 1 > /etc/vextract-ocr-version \
    && chmod 0444 /etc/vextract-engine-version /etc/vextract-ocr-version

COPY --from=build /out/vextract /usr/local/bin/vextract

# Numeric unprivileged user; no home directory or shell is needed. The server
# also runs the container read-only with a small /tmp tmpfs.
USER 65532:65532

ENTRYPOINT ["/usr/local/bin/vextract"]
