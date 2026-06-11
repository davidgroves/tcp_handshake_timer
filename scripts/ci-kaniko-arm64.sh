#!/bin/sh
set -eu

repo="${1:?repo base name required (e.g. tcp_handshake_timer)}"
tag="${CI_COMMIT_TAG:?CI_COMMIT_TAG is required}"
token="${REGISTRY_TOKEN:?REGISTRY_TOKEN is required}"
context="${CI_WORKSPACE:-/woodpecker/src}"

mkdir -p /kaniko/.docker
auth=$(printf '%s:%s' "${CI_REPO_OWNER}" "$token" | base64 | tr -d '\n')
printf '{"auths":{"codeberg.org":{"auth":"%s"}}}' "$auth" > /kaniko/.docker/config.json

/kaniko/executor \
  --context "dir://${context}" \
  --dockerfile Dockerfile \
  --custom-platform=linux/arm64 \
  --build-arg "TARGETARCH=arm64" \
  --destination "codeberg.org/${CI_REPO_OWNER}/${repo}-arm64:${tag}"
