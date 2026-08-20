#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

requireIntegrationGate() {
    if [ "${ISOLARIUM_EC2_INTEGRATION:-}" != "1" ]; then
        echo "FAIL: ISOLARIUM_EC2_INTEGRATION=1 is required to run EC2 tests"
        exit 1
    fi
}

runEC2Tests() {
    local logFile="$1"
    go test -v -tags=ec2 -timeout 30m ./internal/ec2/... 2>&1 | tee "$logFile"
    return "${PIPESTATUS[0]}"
}

failWhenGoTestSelectedNothing() {
    local logFile="$1"
    if grep -q "no tests to run" "$logFile"; then
        echo "FAIL: go test reported 'no tests to run' - the ec2 build tag selected no test"
        exit 1
    fi
}

failWhenNoTestExecuted() {
    local logFile="$1"
    if ! grep -q "^=== RUN" "$logFile"; then
        echo "FAIL: no EC2 test executed"
        exit 1
    fi
}

requireIntegrationGate

echo "=== Running EC2 tests against a real AWS account ==="

LOG_FILE="$(mktemp)"
trap 'rm -f "$LOG_FILE"' EXIT

TEST_STATUS=0
runEC2Tests "$LOG_FILE" || TEST_STATUS=$?

failWhenGoTestSelectedNothing "$LOG_FILE"
failWhenNoTestExecuted "$LOG_FILE"

if [ "$TEST_STATUS" -ne 0 ]; then
    echo "FAIL: go test -tags=ec2 exited $TEST_STATUS"
    exit "$TEST_STATUS"
fi

echo "=== EC2 tests passed ==="
