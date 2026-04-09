#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

export APP_ENV="${APP_ENV:-dev}"
export DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@localhost:5435/eighty_twenty_ops?sslmode=disable}"
export PORT="${PORT:-3201}"
export FRONTEND_ORIGIN="${FRONTEND_ORIGIN:-http://localhost:3200}"
export SESSION_SECRET="${SESSION_SECRET:-eighty-twenty-local-dev-secret}"

go run ./cmd/server
