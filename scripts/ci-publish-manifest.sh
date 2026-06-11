#!/bin/sh
set -eu

repo="${1:?repo base name required}"
tag="${CI_COMMIT_TAG:?CI_COMMIT_TAG is required}"
token="${REGISTRY_TOKEN:?REGISTRY_TOKEN is required}"
ver="${tag#v}"

major=$(echo "$ver" | cut -d. -f1)
minor=$(echo "$ver" | cut -d. -f2)
manifest_tags="${tag},latest,${ver},${major}.${minor},${major}"

curl -fsSL \
  "https://github.com/estesp/manifest-tool/releases/download/v2.1.5/manifest-tool-v2.1.5.tar.gz" \
  | tar xz
install -m 755 manifest-tool-linux-amd64 /usr/local/bin/manifest-tool

manifest-tool \
  --username "${CI_REPO_OWNER}" \
  --password "${token}" \
  push from-args \
  --platforms "linux/amd64,linux/arm64" \
  --template "codeberg.org/${CI_REPO_OWNER}/${repo}-ARCH:${tag}" \
  --target "codeberg.org/${CI_REPO_OWNER}/${repo}" \
  --tags "${manifest_tags}"
