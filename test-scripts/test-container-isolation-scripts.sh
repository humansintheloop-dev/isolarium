#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Testing container isolation_scripts ==="

CONTAINER_NAME="isolarium-test-isolation-scripts"
TEST_FIXTURE="$PROJECT_ROOT/testdata/pid-yaml-test"

if ! docker info &> /dev/null; then
    echo "SKIP: Docker not available, skipping container isolation_scripts test"
    exit 0
fi

cleanup() {
    echo "--- Cleaning up test container ---"
    ./bin/isolarium destroy --type container --name "$CONTAINER_NAME" 2>/dev/null || true
}

trap cleanup EXIT

echo "--- Building isolarium ---"
go build -o bin/isolarium ./cmd/isolarium

echo "--- Creating container with isolation_scripts ---"
./bin/isolarium create --type container --name "$CONTAINER_NAME" --work-directory "$TEST_FIXTURE"

echo "--- Verifying isolation script ran (marker file exists) ---"
MARKER_CONTENT=$(docker exec "$CONTAINER_NAME" cat /opt/isolarium/marker.txt)

if [ "$MARKER_CONTENT" = "isolation-scripts-ok" ]; then
    echo "PASS: isolation script created marker file with expected content"
else
    echo "FAIL: expected 'isolation-scripts-ok' but got '$MARKER_CONTENT'"
    exit 1
fi

echo "--- Destroying container ---"
./bin/isolarium destroy --type container --name "$CONTAINER_NAME"
trap - EXIT

echo "=== Container isolation_scripts test passed ==="
