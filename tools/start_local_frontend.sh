#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../frontend"

export VITE_DEV_PORT="${VITE_DEV_PORT:-3200}"
export VITE_BACKEND_ORIGIN="${VITE_BACKEND_ORIGIN:-http://localhost:3001}"

npm run dev -- --host 0.0.0.0 --port "${VITE_DEV_PORT}"
