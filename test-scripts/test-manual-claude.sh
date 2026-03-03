#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

FORCE=false
CLEANUP=false
TYPES=()

for arg in "$@"; do
    case "$arg" in
        --force) FORCE=true ;;
        --cleanup) CLEANUP=true ;;
        nono|container|vm|all) TYPES+=("$arg") ;;
        *)
            echo "Usage: $0 [--force] [--cleanup] [nono|container|vm|all]"
            exit 1
            ;;
    esac
done

if [ ${#TYPES[@]} -eq 0 ]; then
    TYPES=("all")
fi

GOTEST_FLAGS=(-v -tags=manual)
if [ "$FORCE" = true ]; then
    GOTEST_FLAGS+=(-count=1)
fi

BINARY="bin/isolarium"
go build -o "$BINARY" ./cmd/isolarium

destroy_environment() {
    local type="$1"
    if [ "$type" = "nono" ]; then
        echo "=== nono has no infrastructure to destroy ==="
        return
    fi
    echo "=== Destroying $type environment ==="
    ./"$BINARY" --type "$type" destroy
}

run_test() {
    local type="$1"
    local test_name="$2"
    echo "=== Running claude hello in $type ==="
    go test "${GOTEST_FLAGS[@]}" -run "$test_name" ./cmd/isolarium/...
}

for TYPE in "${TYPES[@]}"; do
    case "$TYPE" in
        nono)      if [ "$CLEANUP" = true ]; then destroy_environment nono; else run_test nono "TestClaude.*InNono_Manual"; fi ;;
        container) if [ "$CLEANUP" = true ]; then destroy_environment container; else run_test container "TestClaude.*InContainer_Manual"; fi ;;
        vm)        if [ "$CLEANUP" = true ]; then destroy_environment vm; else run_test vm "TestClaudeNonInteractiveInVM_Manual"; fi ;;
        all)
            if [ "$CLEANUP" = true ]; then
                destroy_environment container
                destroy_environment vm
            else
                run_test nono "TestClaude.*InNono_Manual"
                run_test container "TestClaude.*InContainer_Manual"
                run_test vm "TestClaudeNonInteractiveInVM_Manual"
            fi
            ;;
    esac
done
