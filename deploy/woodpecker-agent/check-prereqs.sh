#!/usr/bin/env bash
set -euo pipefail

fail=0

check() {
  if "$@"; then
    echo "ok: $*"
  else
    echo "fail: $*"
    fail=1
  fi
}

check command -v docker
check docker info >/dev/null
check docker compose version >/dev/null
check timeout 5 bash -c 'echo > /dev/tcp/grpc.ci.codeberg.org/443'

if [[ ! -f .env ]]; then
  echo "fail: .env missing (copy .env.example and set WOODPECKER_AGENT_SECRET)"
  fail=1
elif [[ -z "${WOODPECKER_AGENT_SECRET:-}" ]] && ! grep -q '^WOODPECKER_AGENT_SECRET=.\+' .env; then
  echo "fail: WOODPECKER_AGENT_SECRET is empty in .env"
  fail=1
else
  echo "ok: .env present with agent secret"
fi

exit "$fail"
