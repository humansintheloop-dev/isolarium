#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

loadEnvLocalIfPresent() {
    if [ -f ".env.local" ]; then
        set -a
        # shellcheck source=/dev/null
        . ".env.local"
        set +a
    fi
}

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

loadEnvLocalIfPresent

echo "=== Testing pre-commit runs all hooks in nono ==="

verifyRequiredSecretsAreSet() {
    if [ -z "${CS_ACCESS_TOKEN:-}" ] || [ -z "${CS_ACE_ACCESS_TOKEN:-}" ]; then
        echo "FAIL: CS_ACCESS_TOKEN and CS_ACE_ACCESS_TOKEN must be set"
        exit 1
    fi
}

verifyNonoIsAvailable() {
    if ! command -v nono &> /dev/null; then
        echo "FAIL: nono not installed"
        exit 1
    fi
}

verifyRequiredSecretsAreSet
verifyNonoIsAvailable

timePhase "go build" go build -o bin/isolarium ./cmd/isolarium

verifyCodeSceneCanAnalyzeCode() {
    echo "--- Verifying CodeScene can analyze code in nono ---"
    local output
    output=$(./bin/isolarium run --type nono --no-gh-token -- \
        cs check cmd/isolarium/main.go 2>&1)
    echo "$output"
    if ! echo "$output" | grep -q 'Code health score'; then
        echo "FAIL: Expected 'Code health score' in cs check output"
        exit 1
    fi
    echo "CodeScene analysis verified"
}

timePhase "cs check" verifyCodeSceneCanAnalyzeCode

timePhase "pre-commit run" ./bin/isolarium run --type nono --no-gh-token -- \
    pre-commit run --files codescene-precommit.sh

echo "=== Pre-commit in nono test passed ==="
