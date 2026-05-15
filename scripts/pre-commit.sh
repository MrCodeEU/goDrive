#!/usr/bin/env bash
# Pre-commit hook — runs fast local checks before every commit.
# Install: make install-hooks
set -euo pipefail

STAGED=$(git diff --cached --name-only --diff-filter=ACMRT 2>/dev/null)
GO_CHANGED=$(echo "$STAGED" | grep '\.go$' || true)
WEB_CHANGED=$(echo "$STAGED" | grep '^web/' || true)
MOBILE_CHANGED=$(echo "$STAGED" | grep '^mobile/' || true)

if [ -n "$GO_CHANGED" ]; then
    echo "→ gofmt..."
    BAD=$(gofmt -l $GO_CHANGED)
    if [ -n "$BAD" ]; then
        echo "FAIL: unformatted Go files (run: gofmt -w <file> or make fmt):"
        echo "$BAD"
        exit 1
    fi

    echo "→ go vet..."
    GOCACHE=/tmp/godrive-gocache go vet ./...

    echo "→ go test..."
    GOCACHE=/tmp/godrive-gocache go test ./...
fi

if [ -n "$WEB_CHANGED" ]; then
    echo "→ svelte-check..."
    npm run check --prefix web --silent
fi

if [ -n "$MOBILE_CHANGED" ]; then
    echo "→ flutter analyze..."
    (cd mobile && flutter analyze)
fi

echo "✓ pre-commit OK"
