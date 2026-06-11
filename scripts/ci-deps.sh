#!/usr/bin/env bash
set -euo pipefail

apt-get update
apt-get install -y --no-install-recommends \
    clang curl llvm libbpf-dev gcc-aarch64-linux-gnu
