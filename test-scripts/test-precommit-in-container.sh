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

echo "=== Testing pre-commit runs all hooks in container ==="

CONTAINER_NAME="isolarium-test-precommit"

verifyRequiredSecretsAreSet() {
    if [ -z "${CS_ACCESS_TOKEN:-}" ] || [ -z "${CS_ACE_ACCESS_TOKEN:-}" ]; then
        echo "FAIL: CS_ACCESS_TOKEN and CS_ACE_ACCESS_TOKEN must be set"
        exit 1
    fi
}

verifyDockerIsAvailable() {
    if ! docker info &> /dev/null; then
        echo "SKIP: Docker not available, skipping pre-commit test"
        exit 0
    fi
}

cleanup() {
    echo "--- Cleaning up test container ---"
    ./bin/isolarium destroy --type container --name "$CONTAINER_NAME" 2>/dev/null || true
}

verifyRequiredSecretsAreSet
verifyDockerIsAvailable

trap cleanup EXIT

timePhase "go build" go build -o bin/isolarium ./cmd/isolarium

timePhase "isolarium create" ./bin/isolarium create --type container --name "$CONTAINER_NAME" --work-directory "$PROJECT_ROOT"

verifyCodeSceneCanAnalyzeCode() {
    echo "--- Verifying CodeScene can analyze code inside container ---"
    local output
    output=$(./bin/isolarium run --type container --name "$CONTAINER_NAME" --copy-session=false --no-gh-token -- \
        cs check cmd/isolarium/main.go 2>&1)
    echo "$output"
    if ! echo "$output" | grep -q 'Code health score'; then
        echo "FAIL: Expected 'Code health score' in cs check output"
        exit 1
    fi
    echo "CodeScene analysis verified"
}

timePhase "cs check" verifyCodeSceneCanAnalyzeCode

timePhase "make file change" ./bin/isolarium run --type container --name "$CONTAINER_NAME" --copy-session=false --no-gh-token -- \
    sh -c 'echo "// harmless test change" >> cmd/isolarium/main.go'

timePhase "pre-commit run --all-files" ./bin/isolarium run --type container --name "$CONTAINER_NAME" --copy-session=false --no-gh-token -- \
    pre-commit run --all-files

timePhase "isolarium destroy" ./bin/isolarium destroy --type container --name "$CONTAINER_NAME"
trap - EXIT

echo "=== Pre-commit in container test passed ==="
