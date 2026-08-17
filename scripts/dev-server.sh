#!/usr/bin/env bash
# Dev-only launcher for board-server: prepares an ephemeral data dir, ensures a
# dev admin password exists, then runs the server. NOT for production.
set -euo pipefail
cd "$(dirname "$0")/.."

export ABP_DATA_DIR="${ABP_DATA_DIR:-.dev-data}"
export ABP_LISTEN_ADDR="${ABP_LISTEN_ADDR:-127.0.0.1:8080}"
export ABP_SECURE_COOKIES="${ABP_SECURE_COOKIES:-false}"
export ABP_PUBLIC_URL="${ABP_PUBLIC_URL:-http://127.0.0.1:8080}"
export ABP_LOG_LEVEL="${ABP_LOG_LEVEL:-info}"

mkdir -p "$ABP_DATA_DIR"

# Idempotently (re)set a dev admin password. set-password overwrites safely.
DEV_PW="${ABP_DEV_ADMIN_PASSWORD:-devpassword123}"
echo "$DEV_PW" | go run ./cmd/board-server admin set-password --password-stdin

echo "board-server: dev admin password = ${DEV_PW} (dev only)"
exec go run ./cmd/board-server run
