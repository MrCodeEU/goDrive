#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
E2E_DIR="$ROOT_DIR/var/e2e"
DATA_ROOT="$E2E_DIR/data"
APPDATA_DIR="$E2E_DIR/appdata"
ADMIN_ROOT="$DATA_ROOT/admin"

rm -rf "$E2E_DIR"
mkdir -p "$ADMIN_ROOT/Documents" "$APPDATA_DIR"

cat > "$ADMIN_ROOT/README-e2e.md" <<'EOF'
# goDrive E2E Fixture

This file is created by the Playwright end-to-end test backend.
EOF

cat > "$ADMIN_ROOT/Documents/seed-note.txt" <<'EOF'
Seeded text file for browsing and preview smoke tests.
EOF

cd "$ROOT_DIR"

export GODRIVE_ADDR="${GODRIVE_E2E_ADDR:-127.0.0.1:18121}"
export GODRIVE_DATA_ROOT="$DATA_ROOT"
export GODRIVE_APPDATA_DIR="$APPDATA_DIR"
export GODRIVE_DB_PATH="$APPDATA_DIR/godrive.sqlite"
export GODRIVE_BOOTSTRAP_ADMIN_USER="${GODRIVE_E2E_USER:-admin}"
export GODRIVE_BOOTSTRAP_ADMIN_PASSWORD="${GODRIVE_E2E_PASSWORD:-change-me-e2e}"
export GODRIVE_BOOTSTRAP_ADMIN_ROOT="$ADMIN_ROOT"
export GODRIVE_DEV_LATENCY=
export GODRIVE_ENABLE_WATCHER=true
export GODRIVE_RECONCILE_INTERVAL=0
export GODRIVE_SEARCH_ENGINE=sqlite
export CCACHE_DISABLE=1
export GOCACHE="${GODRIVE_E2E_GOCACHE:-/tmp/godrive-e2e-gocache}"

exec go run ./cmd/godrive
