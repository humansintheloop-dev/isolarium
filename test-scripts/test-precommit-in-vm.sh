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

loadEnvLocalIfPresent

echo "=== Testing pre-commit runs all hooks in VM ==="

if ! command -v limactl &> /dev/null; then
    echo "SKIP: Lima not installed, skipping pre-commit in VM test"
    exit 0
fi

VM_NAME="isolarium-test-precommit"

verifyRequiredSecretsAreSet() {
    if [ -z "${CS_ACCESS_TOKEN:-}" ] || [ -z "${CS_ACE_ACCESS_TOKEN:-}" ]; then
        echo "FAIL: CS_ACCESS_TOKEN and CS_ACE_ACCESS_TOKEN must be set"
        exit 1
    fi
}

cleanup() {
    echo "--- Cleaning up test VM ---"
    ./bin/isolarium destroy --type vm --name "$VM_NAME" 2>/dev/null || true
}

verifyRequiredSecretsAreSet

trap cleanup EXIT

echo "--- Building isolarium ---"
go build -o bin/isolarium ./cmd/isolarium

echo "--- Creating VM for isolarium repo ---"
./bin/isolarium create --type vm --name "$VM_NAME"

echo "--- Syncing local test scripts to VM ---"
syncLocalChangesToVM() {
    local vm_home
    # shellcheck disable=SC2016
    vm_home=$(limactl shell "$VM_NAME" -- bash -c 'echo $HOME')
    local vm_repo="$vm_home/repo"
    for f in test-scripts/*.sh; do
        limactl copy "$f" "$VM_NAME:$vm_repo/$f"
    done
}
syncLocalChangesToVM

echo "--- Making a harmless file change inside VM ---"
./bin/isolarium run --type vm --name "$VM_NAME" --copy-session=false --no-gh-token -- \
    sh -c 'echo "// harmless test change" >> cmd/isolarium/main.go'

echo "--- Running pre-commit run --all-files with codescene tokens ---"
./bin/isolarium --env CS_ACCESS_TOKEN --env CS_ACE_ACCESS_TOKEN run --type vm --name "$VM_NAME" --copy-session=false --no-gh-token -- \
    pre-commit run --all-files

echo "--- Destroying VM ---"
./bin/isolarium destroy --type vm --name "$VM_NAME"
trap - EXIT

echo "=== Pre-commit in VM test passed ==="
