#!/usr/bin/env bash
# perf-test.sh — 400k-file performance smoke test for goDrive
#
# Steps:
#   1. Seed test data (100k images + 300k text files) unless already present
#   2. Start goDrive server against the test data
#   3. Wait for health check to pass
#   4. Authenticate (get bearer token)
#   5. Benchmark: full reindex, listing, search, paginated list
#   6. Stop server
#   7. Print results table
#
# Usage:
#   make perf-test
#   scripts/perf-test.sh [data_dir]
#
# Env overrides:
#   PERF_DATA_DIR         (default ./var/perf-data)
#   PERF_ADDR             (default 127.0.0.1:18121)
#   PERF_ADMIN_USER       (default perf-admin)
#   PERF_ADMIN_PASS       (default perf-pass-12345)
#   PERF_IMAGE_COUNT      (default 100000)
#   PERF_TEXT_COUNT       (default 300000)
#   SKIP_SEED             set to 1 to skip data generation
#   SKIP_WARMUP           set to 1 to skip preview warmup benchmark

set -euo pipefail
cd "$(dirname "$0")/.."

DATA_DIR="${PERF_DATA_DIR:-./var/perf-data}"
APPDATA_DIR="${PERF_APPDATA_DIR:-./var/perf-appdata}"
ADDR="${PERF_ADDR:-127.0.0.1:18121}"
ADMIN_USER="${PERF_ADMIN_USER:-perf-admin}"
ADMIN_PASS="${PERF_ADMIN_PASS:-perf-pass-12345}"
BASE_URL="http://${ADDR}"
SERVER_PID=""
RESULTS=()

log()  { echo "[perf] $*"; }
fail() { echo "[perf] ERROR: $*" >&2; exit 1; }

cleanup() {
    if [[ -n "$SERVER_PID" ]]; then
        log "Stopping server (pid $SERVER_PID)..."
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# 1. Build binary
# ---------------------------------------------------------------------------
log "Building godrive binary..."
CCACHE_DISABLE=1 go build -o ./godrive ./cmd/godrive

# ---------------------------------------------------------------------------
# 2. Seed test data
# ---------------------------------------------------------------------------
if [[ "${SKIP_SEED:-0}" != "1" ]]; then
    log "Seeding test data → $DATA_DIR"
    python3 scripts/perf-seed.py "$DATA_DIR" \
        --images "${PERF_IMAGE_COUNT:-100000}" \
        --text   "${PERF_TEXT_COUNT:-300000}"
else
    log "Skipping seed (SKIP_SEED=1)"
    [[ -d "$DATA_DIR" ]] || fail "SKIP_SEED=1 but $DATA_DIR does not exist"
fi

# ---------------------------------------------------------------------------
# 3. Start server
# ---------------------------------------------------------------------------
log "Starting goDrive server on $ADDR..."
mkdir -p "$APPDATA_DIR"

env \
    GODRIVE_ADDR="$ADDR" \
    GODRIVE_DATA_ROOT="$(realpath "$DATA_DIR")" \
    GODRIVE_APPDATA_DIR="$(realpath "$APPDATA_DIR")" \
    GODRIVE_DB_PATH="$(realpath "$APPDATA_DIR")/perf.sqlite" \
    GODRIVE_BOOTSTRAP_ADMIN_USER="$ADMIN_USER" \
    GODRIVE_BOOTSTRAP_ADMIN_PASSWORD="$ADMIN_PASS" \
    GODRIVE_BOOTSTRAP_ADMIN_ROOT="$(realpath "$DATA_DIR")" \
    GODRIVE_COOKIE_SECURE=false \
    GODRIVE_ENABLE_WATCHER=false \
    GODRIVE_PREVIEW_WORKERS=0 \
    GODRIVE_DEV_LATENCY="" \
    ./godrive serve >"$APPDATA_DIR/server.log" 2>&1 &
SERVER_PID=$!

# ---------------------------------------------------------------------------
# 4. Wait for health
# ---------------------------------------------------------------------------
log "Waiting for server to be ready..."
for i in $(seq 1 30); do
    if curl -sf "$BASE_URL/health" >/dev/null 2>&1; then
        log "Server ready (${i}s)"
        break
    fi
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        fail "Server exited early. Log:\n$(cat "$APPDATA_DIR/server.log")"
    fi
    sleep 1
    if [[ $i -eq 30 ]]; then
        fail "Server did not become healthy after 30s"
    fi
done

# ---------------------------------------------------------------------------
# 5. Authenticate
# ---------------------------------------------------------------------------
log "Authenticating as $ADMIN_USER..."
TOKEN=$(curl -sf -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")
AUTH="Authorization: Bearer $TOKEN"
log "Token acquired."

# ---------------------------------------------------------------------------
# Helper: time an HTTP GET, record result
# ---------------------------------------------------------------------------
time_get() {
    local label="$1"
    local url="$2"
    local start end elapsed http_code

    start=$(date +%s%3N)
    http_code=$(curl -sf -o /dev/null -w "%{http_code}" \
        -H "$AUTH" "$url" 2>/dev/null || echo "000")
    end=$(date +%s%3N)
    elapsed=$(( end - start ))

    if [[ "$http_code" != "200" ]]; then
        RESULTS+=("$(printf "%-45s  FAIL (HTTP %s)" "$label" "$http_code")")
    else
        RESULTS+=("$(printf "%-45s  %5d ms" "$label" "$elapsed")")
    fi
    log "$label → ${elapsed}ms (HTTP $http_code)"
}

# ---------------------------------------------------------------------------
# Helper: time a CLI command
# ---------------------------------------------------------------------------
time_cli() {
    local label="$1"
    shift
    local start end elapsed

    start=$(date +%s%3N)
    env \
        GODRIVE_DATA_ROOT="$(realpath "$DATA_DIR")" \
        GODRIVE_APPDATA_DIR="$(realpath "$APPDATA_DIR")" \
        GODRIVE_DB_PATH="$(realpath "$APPDATA_DIR")/perf.sqlite" \
        GODRIVE_BOOTSTRAP_ADMIN_USER="$ADMIN_USER" \
        GODRIVE_BOOTSTRAP_ADMIN_PASSWORD="$ADMIN_PASS" \
        GODRIVE_BOOTSTRAP_ADMIN_ROOT="$(realpath "$DATA_DIR")" \
        ./godrive "$@" >/dev/null 2>&1
    end=$(date +%s%3N)
    elapsed=$(( end - start ))

    RESULTS+=("$(printf "%-45s  %5d ms" "$label" "$elapsed")")
    log "$label → ${elapsed}ms"
}

# ---------------------------------------------------------------------------
# 6. Benchmarks
# ---------------------------------------------------------------------------

log "--- Benchmarks ---"

# List root (first page)
time_get "list / (first page, limit=200)"       "$BASE_URL/api/files/list?path=/&limit=200"

# List root (limit=50, simulates mobile)
time_get "list / (mobile, limit=50)"            "$BASE_URL/api/files/list?path=/&limit=50"

# List a subdirectory
time_get "list /images/00 (subdir)"             "$BASE_URL/api/files/list?path=/images/00&limit=200"

# Search — common word that will hit many results
time_get "search 'alpha' (indexed)"             "$BASE_URL/api/files/search?q=alpha&limit=50"
time_get "search 'photo' (indexed)"             "$BASE_URL/api/files/search?q=photo&limit=50"
time_get "search 'img_0000001' (exact)"         "$BASE_URL/api/files/search?q=img_0000001&limit=10"

# Status via CLI (reads DB stats)
time_cli "godrive status"                       status

# ---------------------------------------------------------------------------
# Full reindex via API so progress is visible while running.
# ---------------------------------------------------------------------------
log "Starting full reindex via API (this may take several minutes for ~830k entries)..."
REINDEX_START=$(date +%s%3N)
curl -sf -X POST "$BASE_URL/api/admin/jobs/reindex" \
    -H "$AUTH" -H "Content-Type: application/json" -d '{}' >/dev/null

while true; do
    JOB=$(curl -sf -H "$AUTH" "$BASE_URL/api/admin/jobs/current" 2>/dev/null || echo '{"job":{}}')
    STATUS=$(echo "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin).get('job',{}); print(d.get('status',''))" 2>/dev/null || echo '')
    DONE=$(echo  "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin).get('job',{}); print(d.get('done',0))" 2>/dev/null || echo '0')
    TOTAL=$(echo "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin).get('job',{}); print(d.get('total',0))" 2>/dev/null || echo '0')
    KNOWN=$(echo "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin).get('job',{}); print(d.get('total_known',False))" 2>/dev/null || echo 'False')
    MSG=$(echo   "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin).get('job',{}); print(d.get('message',''))" 2>/dev/null || echo '')

    if [[ "$KNOWN" == "True" && "$TOTAL" -gt 0 ]]; then
        PCT=$(( DONE * 100 / TOTAL ))
        NOW=$(date +%s%3N)
        ELAPSED_MS=$(( NOW - REINDEX_START ))
        ETA_STR=""
        if [[ "$DONE" -gt 0 && "$ELAPSED_MS" -gt 1000 ]]; then
            REMAIN=$(python3 -c "import sys; d,t,e=int(sys.argv[1]),int(sys.argv[2]),int(sys.argv[3]); r=round((t-d)/(d/(e/1000))); s=r%60; m=r//60; h=m//60; rm=m%60; print(f'~{h}h{rm}m' if h>0 else (f'~{m}m{s}s' if m>0 and s>0 else (f'~{m}m' if m>0 else f'~{s}s')))" "$DONE" "$TOTAL" "$ELAPSED_MS" 2>/dev/null || echo "")
            [[ -n "$REMAIN" ]] && ETA_STR=" ETA $REMAIN"
        fi
        printf "\r[perf] reindex: %d / %d entries (%d%%)%s — %s  " "$DONE" "$TOTAL" "$PCT" "$ETA_STR" "$MSG"
    else
        printf "\r[perf] reindex: %d entries — %s  " "$DONE" "$MSG"
    fi

    if [[ "$STATUS" == "completed" || "$STATUS" == "failed" || "$STATUS" == "canceled" ]]; then
        echo ""
        break
    fi
    sleep 3
done

REINDEX_END=$(date +%s%3N)
REINDEX_ELAPSED=$(( REINDEX_END - REINDEX_START ))
if [[ "$STATUS" == "completed" ]]; then
    RESULTS+=("$(printf "%-45s  %5d ms" "reindex (full, ~830k entries)" "$REINDEX_ELAPSED")")
    log "reindex completed → ${REINDEX_ELAPSED}ms"
else
    RESULTS+=("$(printf "%-45s  FAIL (%s)" "reindex (full, ~830k entries)" "$STATUS")")
    log "reindex ended with status: $STATUS — $MSG"
fi

# List again post-reindex (verify index is populated)
time_get "list / post-reindex"                  "$BASE_URL/api/files/list?path=/&limit=200"
time_get "search 'alpha' post-reindex"          "$BASE_URL/api/files/search?q=alpha&limit=50"

# Preview warmup (optional — slow, tests thumbnail generation throughput)
if [[ "${SKIP_WARMUP:-0}" != "1" ]]; then
    log "Running preview warmup via API (SKIP_WARMUP=1 to skip)..."
    WARMUP_START=$(date +%s%3N)
    curl -sf -X POST "$BASE_URL/api/admin/jobs/preview-warmup" \
        -H "$AUTH" -H "Content-Type: application/json" -d '{}' >/dev/null

    while true; do
        JOB=$(curl -sf -H "$AUTH" "$BASE_URL/api/admin/jobs/current" 2>/dev/null || echo '{"job":{}}')
        STATUS=$(echo "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin).get('job',{}); print(d.get('status',''))" 2>/dev/null || echo '')
        DONE=$(echo  "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin).get('job',{}); print(d.get('done',0))" 2>/dev/null || echo '0')
        TOTAL=$(echo "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin).get('job',{}); print(d.get('total',0))" 2>/dev/null || echo '0')
        KNOWN=$(echo "$JOB" | python3 -c "import sys,json; d=json.load(sys.stdin).get('job',{}); print(d.get('total_known',False))" 2>/dev/null || echo 'False')

        if [[ "$KNOWN" == "True" && "$TOTAL" -gt 0 ]]; then
            PCT=$(( DONE * 100 / TOTAL ))
            NOW=$(date +%s%3N)
            ELAPSED_MS=$(( NOW - WARMUP_START ))
            ETA_STR=""
            if [[ "$DONE" -gt 0 && "$ELAPSED_MS" -gt 1000 ]]; then
                REMAIN=$(python3 -c "import sys; d,t,e=int(sys.argv[1]),int(sys.argv[2]),int(sys.argv[3]); r=round((t-d)/(d/(e/1000))); s=r%60; m=r//60; h=m//60; rm=m%60; print(f'~{h}h{rm}m' if h>0 else (f'~{m}m{s}s' if m>0 and s>0 else (f'~{m}m' if m>0 else f'~{s}s')))" "$DONE" "$TOTAL" "$ELAPSED_MS" 2>/dev/null || echo "")
                [[ -n "$REMAIN" ]] && ETA_STR=" ETA $REMAIN"
            fi
            printf "\r[perf] preview-warmup: %d / %d (%d%%)%s — %s  " "$DONE" "$TOTAL" "$PCT" "$ETA_STR" "$STATUS"
        else
            printf "\r[perf] preview-warmup: %d done — %s  " "$DONE" "$STATUS"
        fi

        if [[ "$STATUS" == "completed" || "$STATUS" == "failed" || "$STATUS" == "canceled" ]]; then
            echo ""
            break
        fi
        sleep 3
    done

    WARMUP_END=$(date +%s%3N)
    WARMUP_ELAPSED=$(( WARMUP_END - WARMUP_START ))
    RESULTS+=("$(printf "%-45s  %5d ms" "preview-warmup" "$WARMUP_ELAPSED")")
    log "preview-warmup → ${WARMUP_ELAPSED}ms ($STATUS)"
fi

# ---------------------------------------------------------------------------
# 7. Results table
# ---------------------------------------------------------------------------
echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║            goDrive 400k-file performance results          ║"
echo "╠══════════════════════════════════════════════════════════╣"
for r in "${RESULTS[@]}"; do
    echo "║  $r  ║"
done
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
log "Server log: $APPDATA_DIR/server.log"
