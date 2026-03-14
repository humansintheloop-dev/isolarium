#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Running Docker integration tests ==="

if docker info &> /dev/null; then
    go test -tags=integration ./internal/docker/...
else
    echo "SKIP: Docker not available, skipping Docker integration tests"
fi

echo ""
"$SCRIPT_DIR/test-container-isolation-scripts.sh"

echo ""
"$SCRIPT_DIR/test-env-flag.sh"

echo ""
"$SCRIPT_DIR/test-host-scripts.sh"

echo ""
"$SCRIPT_DIR/test-precommit-in-container.sh"
