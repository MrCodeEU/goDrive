#!/usr/bin/env bash
# Pre-commit hook — auto-fixes formatting then verifies.
# Install: make install-hooks
set -euo pipefail

STAGED=$(git diff --cached --name-only --diff-filter=ACMRT 2>/dev/null)
GO_CHANGED=$(echo "$STAGED" | grep '\.go$' || true)
WEB_CHANGED=$(echo "$STAGED" | grep '^web/' || true)
MOBILE_CHANGED=$(echo "$STAGED" | grep '^mobile/' || true)
DART_CHANGED=$(echo "$STAGED" | grep '\.dart$' || true)

if [ -n "$GO_CHANGED" ]; then
    echo "→ gofmt (auto-fix)..."
    # shellcheck disable=SC2086
    gofmt -w $GO_CHANGED
    # shellcheck disable=SC2086
    git add $GO_CHANGED

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
    if [ -n "$DART_CHANGED" ]; then
        echo "→ dart format (auto-fix)..."
        # Paths are repo-relative; dart format runs from repo root.
        # shellcheck disable=SC2086
        dart format $DART_CHANGED
        # shellcheck disable=SC2086
        git add $DART_CHANGED
    fi

    echo "→ flutter analyze..."
    (cd mobile && flutter analyze)

    echo "→ flutter test..."
    (cd mobile && flutter test)
fi

echo "✓ pre-commit OK"
