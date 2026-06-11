#!/usr/bin/env bash
set -euo pipefail

tag="${CI_COMMIT_TAG:?CI_COMMIT_TAG is required}"
dist="${1:-dist}"

mkdir -p "$dist"
make generate

for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    go build -buildvcs=false -tags linux -ldflags="-s -w" \
    -o "${dist}/thtimer-${tag}-linux-${arch}" ./cmd/thtimer
done
