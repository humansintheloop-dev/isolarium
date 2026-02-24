#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Running cleanup ==="

if command -v limactl &> /dev/null; then
    go test -tags=cleanup -run TestDestroyCommand ./...
else
    echo "SKIP: Lima not installed, skipping cleanup"
fi
