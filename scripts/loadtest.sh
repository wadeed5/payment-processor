#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> loadtest (requires postgres + nats; stop wallet-service to avoid duplicate handlers)"
echo "    docker compose up -d postgres nats"
echo "    docker compose stop wallet-service   # optional but recommended"
echo ""

go run ./cmd/loadtest "$@"
