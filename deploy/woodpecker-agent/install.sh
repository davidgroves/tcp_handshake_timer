#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "Created .env from .env.example"
  echo "Edit $ROOT/.env and set WOODPECKER_AGENT_SECRET from:"
  echo "  https://ci.codeberg.org -> user settings -> Agents -> Add agent"
  exit 1
fi

./check-prereqs.sh
docker compose up -d
docker compose ps
echo "Agent started. Confirm it appears as connected at https://ci.codeberg.org (user settings -> Agents)."
