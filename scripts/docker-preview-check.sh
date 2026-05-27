#!/usr/bin/env bash
# docker-preview-check.sh — verify preview tools inside the production Docker image
#
# Builds (or reuses) the production Docker image, runs `godrive verify` inside it,
# then functionally tests each preview tool against a sample file.
#
# Usage:
#   scripts/docker-preview-check.sh
#   scripts/docker-preview-check.sh --no-build   # skip docker build, use existing image
#
# Exit code: 0 = all required tools pass, 1 = any failure

set -euo pipefail
cd "$(dirname "$0")/.."

# Prefer podman if available (Fedora/RHEL). Podman tags with localhost/ prefix.
if command -v podman >/dev/null 2>&1; then
    DOCKER=podman
    IMAGE="localhost/godrive:preview-check"
else
    DOCKER=docker
    IMAGE="godrive:preview-check"
fi
CONTAINER_DATA="/data"
CONTAINER_APPDATA="/appdata"
PASS=0
FAIL=0
WARN=0
RESULTS=()

log()  { echo "[docker-preview-check] $*"; }
ok()   { PASS=$(( PASS + 1 )); RESULTS+=("$(printf "%-40s  PASS" "$1")"); log "PASS: $1"; }
fail() { FAIL=$(( FAIL + 1 )); RESULTS+=("$(printf "%-40s  FAIL  %s" "$1" "$2")"); log "FAIL: $1 — $2"; }
warn() { WARN=$(( WARN + 1 )); RESULTS+=("$(printf "%-40s  WARN  %s" "$1" "$2")"); log "WARN: $1 — $2"; }

# ---------------------------------------------------------------------------
# 1. Build image (unless --no-build)
# ---------------------------------------------------------------------------
NO_BUILD=0
for arg in "$@"; do [[ "$arg" == "--no-build" ]] && NO_BUILD=1; done

if [[ "$NO_BUILD" == "0" ]]; then
    log "Building production Docker image ($IMAGE)..."
    $DOCKER build -t "$IMAGE" .
else
    log "Skipping build (--no-build). Using existing image $IMAGE."
fi

# ---------------------------------------------------------------------------
# Helper: run a command inside a fresh container, return output
# ---------------------------------------------------------------------------
docker_run() {
    $DOCKER run --rm \
        -e GODRIVE_DATA_ROOT="$CONTAINER_DATA" \
        -e GODRIVE_APPDATA_DIR="$CONTAINER_APPDATA" \
        -e GODRIVE_DB_PATH="$CONTAINER_APPDATA/godrive.sqlite" \
        -e GODRIVE_UPLOAD_DIR="$CONTAINER_APPDATA/uploads" \
        -e GODRIVE_PREVIEW_DIR="$CONTAINER_APPDATA/previews" \
        -e GODRIVE_TRASH_DIR="$CONTAINER_APPDATA/trash" \
        -e GODRIVE_BOOTSTRAP_ADMIN_USER="check-admin" \
        -e GODRIVE_BOOTSTRAP_ADMIN_PASSWORD="check-pass-12345" \
        -e GODRIVE_BOOTSTRAP_ADMIN_ROOT="$CONTAINER_DATA" \
        -e GODRIVE_COOKIE_SECURE=false \
        "$IMAGE" "$@"
}

# ---------------------------------------------------------------------------
# 2. godrive verify
# ---------------------------------------------------------------------------
log "Running 'godrive verify'..."
VERIFY_OUT=$(docker_run verify 2>&1) || true
echo "$VERIFY_OUT"

if echo "$VERIFY_OUT" | grep -q "^ok: data root"; then
    ok "godrive verify: data root"
else
    fail "godrive verify: data root" "not reported as ok"
fi

if echo "$VERIFY_OUT" | grep -q "^ok: appdata"; then
    ok "godrive verify: appdata"
else
    fail "godrive verify: appdata" "not reported as ok"
fi

for tool in vipsthumbnail ffmpeg pdftoppm libreoffice; do
    if echo "$VERIFY_OUT" | grep -q "^ok: preview tool $tool"; then
        ok "godrive verify: $tool"
    elif echo "$VERIFY_OUT" | grep -q "^warn: preview tool $tool missing"; then
        warn "godrive verify: $tool" "missing from image"
    else
        fail "godrive verify: $tool" "unexpected output"
    fi
done

# prlimit is optional
if echo "$VERIFY_OUT" | grep -q "^ok: preview tool prlimit"; then
    ok "godrive verify: prlimit (optional)"
else
    warn "godrive verify: prlimit (optional)" "not present — preview limits inactive"
fi

# ---------------------------------------------------------------------------
# 3. Functional preview tool tests (run directly inside the image)
# ---------------------------------------------------------------------------

log "--- Functional tool tests ---"

# Helper: run a shell command in the image
docker_sh() {
    $DOCKER run --rm --entrypoint /bin/sh "$IMAGE" -c "$1"
}

# --- vipsthumbnail ---
log "Testing vipsthumbnail (JPEG → JPEG thumbnail)..."
if docker_sh \
    'set -e
     python3 -c "
import struct, zlib
def chunk(n,d): c=struct.pack(\">I\",len(d))+n+d; return c+struct.pack(\">I\",zlib.crc32(n+d)&0xFFFFFFFF)
sig=b\"\x89PNG\r\n\x1a\n\"
ihdr=chunk(b\"IHDR\",struct.pack(\">IIBBBBB\",64,64,8,2,0,0,0))
row=b\"\x00\"+bytes([100,150,200]*64)
raw=row*64
idat=chunk(b\"IDAT\",zlib.compress(raw,1))
iend=chunk(b\"IEND\",b\"\")
open(\"/tmp/test.png\",\"wb\").write(sig+ihdr+idat+iend)
"
     vipsthumbnail /tmp/test.png -o /tmp/thumb.jpg -s 64
     test -s /tmp/thumb.jpg' 2>/dev/null; then
    ok "vipsthumbnail: PNG → JPEG thumbnail"
else
    fail "vipsthumbnail: PNG → JPEG thumbnail" "command failed or empty output"
fi

# --- ffmpeg (image fallback: PNG → JPEG) ---
log "Testing ffmpeg (PNG → JPEG thumbnail fallback)..."
if docker_sh \
    'set -e
     python3 -c "
import struct, zlib
def chunk(n,d): c=struct.pack(\">I\",len(d))+n+d; return c+struct.pack(\">I\",zlib.crc32(n+d)&0xFFFFFFFF)
sig=b\"\x89PNG\r\n\x1a\n\"
ihdr=chunk(b\"IHDR\",struct.pack(\">IIBBBBB\",64,64,8,2,0,0,0))
row=b\"\x00\"+bytes([200,100,50]*64)
raw=row*64
idat=chunk(b\"IDAT\",zlib.compress(raw,1))
iend=chunk(b\"IEND\",b\"\")
open(\"/tmp/test_ff.png\",\"wb\").write(sig+ihdr+idat+iend)
"
     ffmpeg -y -i /tmp/test_ff.png -vf scale=64:64 -frames:v 1 -q:v 5 /tmp/thumb_ff.jpg 2>/dev/null
     test -s /tmp/thumb_ff.jpg' 2>/dev/null; then
    ok "ffmpeg: PNG → JPEG thumbnail"
else
    fail "ffmpeg: PNG → JPEG thumbnail" "command failed or empty output"
fi

# --- ffmpeg (video poster frame) ---
log "Testing ffmpeg (video poster frame)..."
if docker_sh \
    'ffmpeg -y -f lavfi -i "color=c=blue:size=128x128:rate=1" -t 1 /tmp/test.mp4 2>/dev/null &&
     ffmpeg -y -i /tmp/test.mp4 -vf "select=eq(n\,0)" -frames:v 1 /tmp/thumb_vid.jpg 2>/dev/null &&
     test -s /tmp/thumb_vid.jpg' 2>/dev/null; then
    ok "ffmpeg: video poster frame"
else
    fail "ffmpeg: video poster frame" "command failed or empty output"
fi

# --- pdftoppm ---
log "Testing pdftoppm (PDF → PPM)..."
if docker_sh \
    'set -e
     # Minimal valid single-page PDF
     cat > /tmp/test.pdf << '"'"'PDFEOF'"'"'
%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/MediaBox[0 0 72 72]/Parent 2 0 R/Resources<<>>>>endobj
xref
0 4
0000000000 65535 f
0000000009 00000 n
0000000052 00000 n
0000000101 00000 n
trailer<</Size 4/Root 1 0 R>>
startxref
186
%%EOF
PDFEOF
     pdftoppm -r 72 -jpeg /tmp/test.pdf /tmp/pdf_out
     ls /tmp/pdf_out-1.jpg >/dev/null 2>&1 || ls /tmp/pdf_out*.jpg >/dev/null 2>&1
     test -s "$(ls /tmp/pdf_out*.jpg | head -1)"' 2>/dev/null; then
    ok "pdftoppm: PDF → JPEG"
else
    fail "pdftoppm: PDF → JPEG" "command failed or empty output"
fi

# --- libreoffice ---
# Minimal DOCX as base64 — avoids shell/Python quoting nesting inside docker_sh.
_LO_DOCX_B64="UEsDBBQAAAAIAIN4u1w8w8hvqgAAAB0BAAALAAAAX3JlbHMvLnJlbHONz7EKwjAUBdC9XxHebtM6iEjTLiJ0lfoBIXltg0leSKLWv3dxsOLgermcy226xVl2x5gMeQF1WQFDr0gbPwm4DKfNHrq2aM5oZTbk02xCYouzPgmYcw4HzpOa0clUUkC/ODtSdDKnkuLEg1RXOSHfVtWOx08D2oKxFct6LSD2ugY2PAP+w9M4GoVHUjeHPv9Y+WoAG2ScMAt4UNRcv+NycRZ4WzR8dbN9AVBLAwQUAAAACACDeLtchfg3XOcAAACnAQAAEwAAAFtDb250ZW50X1R5cGVzXS54bWx9kMtOwzAQRff9CstbFDuwQAgl6YLHEliUD7CcSWJhz1ieaQh/j9JCkRBlfR9n5jbbJUU1Q+FA2OpLU2sF6KkPOLb6dfdY3ehtt2l2HxlYLSkit3oSybfWsp8gOTaUAZcUByrJCRsqo83Ov7kR7FVdX1tPKIBSydqhu41SzT0Mbh9FPSwCeEQXiKzV3dG74lrtco7BOwmEdsb+F6j6gpgC8eDhKWS+WFLU9hxkFc8zfqLPM5QSelAvrsiTS9Bq+06ltz35fQIU83/TH9fSMAQPp/zalgt5YA44pmhOSnIBv79o7GH47hNQSwMEFAAAAAgAg3i7XGqGXhqdAAAAygAAABEAAAB3b3JkL2RvY3VtZW50LnhtbDXOQQrDIBQE0H1OIe4b0y5KEWM2pfQA7QFStUZQv3xtTG9fDHTzGAYGRkxb8GQ1mB3EkR77gRITFWgX7Uifj9vhQifZico1qE8wsZAt+Jh5HelSSuKMZbWYMOcekolb8G/AMJfcA1pWAXVCUCZnF23w7DQMZxZmF6nsCBGVv0B/pag8NbBR5N14D8TCFd1qBGtVE3fT7j7rWvrfkj9QSwECFAMUAAAACACDeLtcPMPIb6oAAAAdAQAACwAAAAAAAAAAAAAAgAEAAAAAX3JlbHMvLnJlbHNQSwECFAMUAAAACACDeLtchfg3XOcAAACnAQAAEwAAAAAAAAAAAAAAgAHTAAAAW0NvbnRlbnRfVHlwZXNdLnhtbFBLAQIUAxQAAAAIAIN4u1xqhl4anQAAAMoAAAARAAAAAAAAAAAAAACAAesBAAB3b3JkL2RvY3VtZW50LnhtbFBLBQYAAAAAAwADALkAAAC3AgAAAAA="
log "Testing libreoffice (DOCX → PDF headless)..."
if docker_sh \
    "set -e
     mkdir -p /tmp/lo_test
     echo '${_LO_DOCX_B64}' | base64 -d > /tmp/lo_test/test.docx
     libreoffice --headless --convert-to pdf --outdir /tmp/lo_test /tmp/lo_test/test.docx 2>/dev/null
     test -s /tmp/lo_test/test.pdf" 2>/dev/null; then
    ok "libreoffice: DOCX → PDF headless"
else
    fail "libreoffice: DOCX → PDF headless" "command failed or empty output"
fi

# ---------------------------------------------------------------------------
# 4. Results table
# ---------------------------------------------------------------------------
echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║          goDrive Docker preview tool check results        ║"
echo "╠══════════════════════════════════════════════════════════╣"
for r in "${RESULTS[@]}"; do
    echo "║  $r  ║"
done
echo "╠══════════════════════════════════════════════════════════╣"
echo "║  PASS: $PASS   WARN: $WARN   FAIL: $FAIL$(printf '%*s' $(( 43 - ${#PASS} - ${#WARN} - ${#FAIL} )) '')║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

if [[ "$FAIL" -gt 0 ]]; then
    log "FAILED: $FAIL check(s) failed."
    exit 1
fi
log "All required checks passed ($PASS pass, $WARN warn)."
