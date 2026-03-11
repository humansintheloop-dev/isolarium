#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

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

echo "--- Building isolarium ---"
go build -o bin/isolarium ./cmd/isolarium

echo "--- Creating container for isolarium repo ---"
./bin/isolarium create --type container --name "$CONTAINER_NAME" --work-directory "$PROJECT_ROOT"

echo "--- Making a harmless file change inside container ---"
./bin/isolarium run --type container --name "$CONTAINER_NAME" --copy-session=false --no-gh-token -- \
    sh -c 'echo "// harmless test change" >> cmd/isolarium/main.go'

echo "--- Running pre-commit run --all-files with codescene tokens ---"
./bin/isolarium --env CS_ACCESS_TOKEN --env CS_ACE_ACCESS_TOKEN run --type container --name "$CONTAINER_NAME" --copy-session=false --no-gh-token -- \
    pre-commit run --all-files

echo "--- Destroying container ---"
./bin/isolarium destroy --type container --name "$CONTAINER_NAME"
trap - EXIT

echo "=== Pre-commit in container test passed ==="
