#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Running Lima integration tests ==="

if ! command -v limactl &> /dev/null; then
    echo "SKIP: Lima not installed, skipping Lima integration tests"
    exit 0
fi

echo "--- Phase 1: Setup (create VM, install tools, clone repos) ---"
go test -v -tags=integration_setup ./internal/lima/...

echo "--- Phase 2: Verification (ordering-independent tests) ---"
go test -v -tags=integration ./internal/lima/...

echo "--- Phase 3: Teardown (destroy VM) ---"
go test -v -tags=integration_teardown ./internal/lima/...
