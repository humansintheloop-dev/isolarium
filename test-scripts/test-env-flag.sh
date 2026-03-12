#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "--- Building isolarium ---"
go build -o bin/isolarium ./cmd/isolarium

TESTS_RUN=0

# --- Container test ---

testEnvFlagWithContainer() {
    echo "=== Testing --env flag passes variables to container ==="

    CONTAINER_NAME="isolarium-test-env-flag"
    TEST_FIXTURE="$PROJECT_ROOT/testdata/pid-yaml-test"

    if ! docker info &> /dev/null; then
        echo "SKIP: Docker not available, skipping container --env flag test"
        return
    fi

    cleanupContainer() {
        echo "--- Cleaning up test container ---"
        ./bin/isolarium destroy --type container --name "$CONTAINER_NAME" 2>/dev/null || true
    }

    trap cleanupContainer EXIT

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

    TESTS_RUN=$((TESTS_RUN + 1))
}

# --- Nono test ---

testEnvFlagWithNono() {
    echo "=== Testing --env flag passes variables to nono ==="

    if ! command -v nono &> /dev/null; then
        echo "SKIP: nono not installed, skipping nono --env flag test"
        return
    fi

    echo "--- Running printenv with --env TEST_VAR=hello123 via nono ---"
    OUTPUT=$(./bin/isolarium --env TEST_VAR=hello123 run --type nono --no-gh-token -- printenv TEST_VAR)

    if echo "$OUTPUT" | grep -q "hello123"; then
        echo "PASS: TEST_VAR=hello123 is visible inside nono sandbox"
    else
        echo "FAIL: expected output to contain 'hello123' but got: $OUTPUT"
        exit 1
    fi

    TESTS_RUN=$((TESTS_RUN + 1))
}

# --- VM test ---

testEnvFlagWithVM() {
    echo "=== Testing --env flag passes variables to VM ==="

    if ! command -v limactl &> /dev/null; then
        echo "SKIP: Lima not installed, skipping VM --env flag test"
        return
    fi

    VM_NAME="isolarium-test-env-flag"

    cleanupVM() {
        echo "--- Cleaning up test VM ---"
        ./bin/isolarium destroy --type vm --name "$VM_NAME" 2>/dev/null || true
    }

    trap cleanupVM EXIT

    echo "--- Creating VM ---"
    ./bin/isolarium create --type vm --name "$VM_NAME"

    echo "--- Running printenv with --env TEST_VAR=hello123 via VM ---"
    OUTPUT=$(./bin/isolarium --env TEST_VAR=hello123 run --type vm --name "$VM_NAME" --copy-session=false --no-gh-token -- printenv TEST_VAR)

    if echo "$OUTPUT" | grep -q "hello123"; then
        echo "PASS: TEST_VAR=hello123 is visible inside VM"
    else
        echo "FAIL: expected output to contain 'hello123' but got: $OUTPUT"
        exit 1
    fi

    echo "--- Destroying VM ---"
    ./bin/isolarium destroy --type vm --name "$VM_NAME"
    trap - EXIT

    TESTS_RUN=$((TESTS_RUN + 1))
}

testEnvFlagWithContainer
testEnvFlagWithNono
testEnvFlagWithVM

if [ "$TESTS_RUN" -eq 0 ]; then
    echo "FAIL: no --env flag tests ran (Docker, nono, and Lima all unavailable)"
    exit 1
fi

echo "=== --env flag tests passed ($TESTS_RUN isolation type(s) tested) ==="
