#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

# The log outlives the run so that a suite that failed, or one that had to be
# interrupted, can still be read afterwards. logs/ is gitignored.
perRunLogFile() {
    local logDir="$PROJECT_ROOT/logs"
    mkdir -p "$logDir"
    echo "$logDir/test-ec2-e2e-$(date +%Y%m%d-%H%M%S).log"
}

# The instance is reached through the AWS SDK's own credential chain, which the
# test does nothing to configure. Lifting the credentials from the profile the
# AWS CLI already uses saves exporting them by hand, and leaves them empty — so
# the run still fails — when no profile is configured.
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

# -count=1 defeats the test cache, because a cached pass would report an
# end-to-end run that never touched the account.
runEndToEndTest() {
    local logFile="$1"
    go test -v -count=1 -tags=e2e_ec2 -timeout 45m ./cmd/isolarium/... 2>&1 | tee "$logFile"
    return "${PIPESTATUS[0]}"
}

failWhenGoTestSelectedNothing() {
    local logFile="$1"
    if grep -q "no tests to run" "$logFile"; then
        echo "FAIL: go test reported 'no tests to run' - the e2e_ec2 build tag selected no test"
        exit 1
    fi
}

failWhenNoTestExecuted() {
    local logFile="$1"
    if ! grep -q "^=== RUN" "$logFile"; then
        echo "FAIL: no EC2 end-to-end test executed"
        exit 1
    fi
}

# The test asserts that its own instance was terminated, which says nothing about
# one an earlier, abandoned run left behind, so the account itself is the last
# word on whether anything is still billing.
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

exportCredentialsFromTheConfiguredProfile

echo "=== Running the EC2 end-to-end test against a real AWS account ==="

LOG_FILE="$(perRunLogFile)"
echo "=== Logging to $LOG_FILE ==="

TEST_STATUS=0
runEndToEndTest "$LOG_FILE" || TEST_STATUS=$?

failWhenGoTestSelectedNothing "$LOG_FILE"
failWhenNoTestExecuted "$LOG_FILE"
failWhenAnyInstanceIsStillRunning

if [ "$TEST_STATUS" -ne 0 ]; then
    echo "FAIL: go test -tags=e2e_ec2 exited $TEST_STATUS"
    exit "$TEST_STATUS"
fi

echo "=== EC2 end-to-end test passed ==="
