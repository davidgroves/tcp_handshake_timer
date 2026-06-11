#!/usr/bin/env bash
set -euo pipefail

TAG="${CI_COMMIT_TAG#refs/tags/}"
REPO="${CI_REPO_NAME}"

mkdir -p dist
git archive --format=tar.gz --prefix="${REPO}-${TAG}/" -o "dist/${REPO}-${TAG}.tar.gz" HEAD
