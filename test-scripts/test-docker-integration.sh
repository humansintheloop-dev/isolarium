#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

timePhase() {
    local label="$1"
    shift
    local start_time
    start_time=$(date +%s)
    "$@"
    local end_time
    end_time=$(date +%s)
    echo "TIMING: $label took $((end_time - start_time))s"
}

OVERALL_START=$(date +%s)

echo "=== Running Docker integration tests ==="

if docker info &> /dev/null; then
    timePhase "go test -tags=integration" go test -tags=integration ./internal/docker/...
else
    echo "SKIP: Docker not available, skipping Docker integration tests"
fi

echo ""
timePhase "test-container-isolation-scripts" "$SCRIPT_DIR/test-container-isolation-scripts.sh"

echo ""
timePhase "test-host-scripts" "$SCRIPT_DIR/test-host-scripts.sh"

echo ""
timePhase "test-precommit-in-container" "$SCRIPT_DIR/test-precommit-in-container.sh"

OVERALL_END=$(date +%s)
echo ""
echo "TIMING: Total docker integration tests took $((OVERALL_END - OVERALL_START))s"
