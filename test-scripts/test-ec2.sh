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

# The whole suite shares one billable instance, created by the first test that
# needs it and terminated by the last. An optional first argument narrows the run
# to the tests whose names match it, which creates the instance on demand so that
# one test can be re-run after a failure; the default matches them all.
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

# Each instance the run launched announced itself with one TIMING line, so
# counting them says how many the suite paid for. A full run needs exactly one; a
# run narrowed to a pattern may legitimately have needed none, when the pattern
# selected only tests that never touch AWS.
failWhenTheSuiteDidNotShareOneInstance() {
    local logFile="$1"
    local wholeSuite="$2"
    local created
    created="$(grep -c 'TIMING: create' "$logFile" || true)"

    if [ "$created" -gt 1 ]; then
        echo "FAIL: the run created $created instances; the EC2 suite must share one"
        exit 1
    fi
    if [ "$wholeSuite" = "true" ] && [ "$created" -ne 1 ]; then
        echo "FAIL: a full EC2 suite run created $created instances, want exactly 1"
        exit 1
    fi
}

# A test that asserts its own instance is terminated still says nothing about an
# instance an earlier, abandoned run left behind, so the account itself is the
# last word on whether anything is still billing.
failWhenAnyInstanceIsStillRunning() {
    if ! command -v aws > /dev/null 2>&1; then
        echo "FAIL: the aws CLI is required to check that no instance was left running"
        exit 1
    fi

    local running
    running="$(aws ec2 describe-instances \
        --filters Name=tag:ManagedBy,Values=isolarium Name=instance-state-name,Values=running \
        --query 'Reservations[].Instances[].InstanceId' --output text)"
    if [ -n "$running" ]; then
        echo "FAIL: isolarium instances are still running and billing: $running"
        exit 1
    fi
}

requireIntegrationGate
exportCredentialsFromTheConfiguredProfile

echo "=== Running EC2 tests against a real AWS account ==="

LOG_FILE="$(mktemp)"
trap 'rm -f "$LOG_FILE"' EXIT

WHOLE_SUITE=true
if [ $# -gt 0 ]; then
    WHOLE_SUITE=false
fi

TEST_STATUS=0
runEC2Tests "$LOG_FILE" "${1:-}" || TEST_STATUS=$?

failWhenGoTestSelectedNothing "$LOG_FILE"
failWhenNoTestExecuted "$LOG_FILE"
failWhenTheSuiteDidNotShareOneInstance "$LOG_FILE" "$WHOLE_SUITE"
failWhenAnyInstanceIsStillRunning

if [ "$TEST_STATUS" -ne 0 ]; then
    echo "FAIL: go test -tags=ec2 exited $TEST_STATUS"
    exit "$TEST_STATUS"
fi

echo "=== EC2 tests passed ==="
