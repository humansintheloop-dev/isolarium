#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Running Nono integration tests ==="

if command -v nono &> /dev/null; then
    go test -tags=integration ./internal/nono/...
else
    echo "SKIP: Nono not installed, skipping Nono integration tests"
fi
