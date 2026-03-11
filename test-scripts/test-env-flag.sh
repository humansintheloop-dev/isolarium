#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Testing --env flag passes variables to container ==="

CONTAINER_NAME="isolarium-test-env-flag"
TEST_FIXTURE="$PROJECT_ROOT/testdata/pid-yaml-test"

if ! docker info &> /dev/null; then
    echo "SKIP: Docker not available, skipping --env flag test"
    exit 0
fi

cleanup() {
    echo "--- Cleaning up test container ---"
    ./bin/isolarium destroy --type container --name "$CONTAINER_NAME" 2>/dev/null || true
}

trap cleanup EXIT

echo "--- Building isolarium ---"
go build -o bin/isolarium ./cmd/isolarium

echo "--- Creating container ---"
./bin/isolarium create --type container --name "$CONTAINER_NAME" --work-directory "$TEST_FIXTURE"

echo "--- Running printenv with --env TEST_VAR=hello123 ---"
OUTPUT=$(./bin/isolarium --env TEST_VAR=hello123 run --type container --name "$CONTAINER_NAME" --copy-session=false --no-gh-token -- printenv TEST_VAR)

if echo "$OUTPUT" | grep -q "hello123"; then
    echo "PASS: TEST_VAR=hello123 is visible inside the container"
else
    echo "FAIL: expected output to contain 'hello123' but got: $OUTPUT"
    exit 1
fi

echo "--- Destroying container ---"
./bin/isolarium destroy --type container --name "$CONTAINER_NAME"
trap - EXIT

echo "=== --env flag test passed ==="
