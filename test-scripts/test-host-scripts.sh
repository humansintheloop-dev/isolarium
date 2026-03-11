#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Testing container host_scripts ==="

CONTAINER_NAME="isolarium-test-host-scripts"
TEST_FIXTURE="$PROJECT_ROOT/testdata/host-script-test"
MARKER_FILE="/tmp/isolarium-host-script-test-marker"

if ! docker info &> /dev/null; then
    echo "SKIP: Docker not available, skipping host_scripts test"
    exit 0
fi

cleanup() {
    echo "--- Cleaning up ---"
    ./bin/isolarium destroy --type container --name "$CONTAINER_NAME" 2>/dev/null || true
    rm -f "$MARKER_FILE"
}

trap cleanup EXIT

rm -f "$MARKER_FILE"

echo "--- Building isolarium ---"
go build -o bin/isolarium ./cmd/isolarium

echo "--- Creating container with host_scripts ---"
./bin/isolarium create --type container --name "$CONTAINER_NAME" --work-directory "$TEST_FIXTURE"

echo "--- Verifying host script ran (marker file exists on host) ---"
if [ -f "$MARKER_FILE" ]; then
    echo "PASS: host script created marker file on host"
else
    echo "FAIL: marker file $MARKER_FILE does not exist"
    exit 1
fi

echo "--- Destroying container ---"
./bin/isolarium destroy --type container --name "$CONTAINER_NAME"
trap - EXIT
rm -f "$MARKER_FILE"

echo "=== Container host_scripts test passed ==="
