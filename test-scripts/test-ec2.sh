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

# The Go tests require the credentials in the environment so that a missing
# account fails the run rather than skipping it. Lifting them from the profile
# the AWS CLI already uses saves exporting them by hand, and leaves them empty —
# so the tests still fail — when no profile is configured.
exportCredentialsFromTheConfiguredProfile() {
    if [ -n "${AWS_ACCESS_KEY_ID:-}" ] && [ -n "${AWS_SECRET_ACCESS_KEY:-}" ]; then
        return
    fi
    if ! command -v aws > /dev/null 2>&1; then
        return
    fi
    AWS_ACCESS_KEY_ID="$(aws configure get aws_access_key_id || true)"
    AWS_SECRET_ACCESS_KEY="$(aws configure get aws_secret_access_key || true)"
    export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY
}

# Every EC2 test builds its own billable instance, so re-running one after a
# failure should not have to rebuild all of them. An optional first argument
# narrows the run to the tests whose names match it; the default matches them all.
runEC2Tests() {
    local logFile="$1"
    local namePattern="${2:-.}"
    go test -v -tags=ec2 -timeout 60m -run "$namePattern" ./internal/ec2/... 2>&1 | tee "$logFile"
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
exportCredentialsFromTheConfiguredProfile

echo "=== Running EC2 tests against a real AWS account ==="

LOG_FILE="$(mktemp)"
trap 'rm -f "$LOG_FILE"' EXIT

TEST_STATUS=0
runEC2Tests "$LOG_FILE" "${1:-}" || TEST_STATUS=$?

failWhenGoTestSelectedNothing "$LOG_FILE"
failWhenNoTestExecuted "$LOG_FILE"

if [ "$TEST_STATUS" -ne 0 ]; then
    echo "FAIL: go test -tags=ec2 exited $TEST_STATUS"
    exit "$TEST_STATUS"
fi

echo "=== EC2 tests passed ==="
