#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Testing --type ec2 is accepted and routed to EC2Backend ==="

echo "--- Building isolarium ---"
go build -o bin/isolarium ./cmd/isolarium

NOT_IMPLEMENTED_MESSAGE="not yet implemented for --type ec2"

TESTS_RUN=0

captureCreateFailure() {
    if ./bin/isolarium create --type ec2 "$@" > /tmp/isolarium-ec2-preflight.out 2>&1; then
        echo "FAIL: expected 'isolarium create --type ec2 $*' to fail, but it succeeded"
        exit 1
    fi
    cat /tmp/isolarium-ec2-preflight.out
}

assertReachesEC2BackendWithName() {
    local expectedName="$1"
    shift

    echo "--- Creating with 'isolarium create --type ec2 $*' ---"
    OUTPUT="$(captureCreateFailure "$@")"

    if echo "$OUTPUT" | grep -q "unknown environment type"; then
        echo "FAIL: --type ec2 was rejected as an unknown environment type: $OUTPUT"
        exit 1
    fi

    if echo "$OUTPUT" | grep -q "invalid type"; then
        echo "FAIL: --type ec2 was rejected by flag parsing: $OUTPUT"
        exit 1
    fi

    if ! echo "$OUTPUT" | grep -q "create \"$expectedName\": $NOT_IMPLEMENTED_MESSAGE"; then
        echo "FAIL: expected output to contain 'create \"$expectedName\": $NOT_IMPLEMENTED_MESSAGE' but got: $OUTPUT"
        exit 1
    fi

    echo "PASS: reached EC2Backend.Create for name '$expectedName'"
    TESTS_RUN=$((TESTS_RUN + 1))
}

assertReachesEC2BackendWithName "my-work" --name my-work
assertReachesEC2BackendWithName "isolarium-ec2"

if [ "$TESTS_RUN" -eq 0 ]; then
    echo "FAIL: no ec2 preflight assertions ran"
    exit 1
fi

echo "=== ec2 preflight tests passed ($TESTS_RUN assertions) ==="
