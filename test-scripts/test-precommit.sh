#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

# pre-commit sets git environment variables that override auto-detection in
# tests that create temporary repos with git init
unset GIT_INDEX_FILE GIT_DIR GIT_WORK_TREE

echo "=== Compile-checking all build tags ==="

go test -run=^$ -count=1 -tags=integration,integration_setup,integration_teardown,cleanup,e2e_gradlew,e2e_pytest ./...

echo "=== Running Go tests ==="

go test ./... -count=1

runNonoE2eTestsIfAvailable() {
    if ! command -v nono &> /dev/null; then
        echo "=== SKIP nono e2e tests: nono not available ==="
        return
    fi

    echo "=== Running nono e2e-gradlew ==="
    "$SCRIPT_DIR/test-end-to-end-with-gradlew.sh" --force nono

    echo "=== Running nono e2e-pytest ==="
    "$SCRIPT_DIR/test-end-to-end-with-pytest.sh" --force nono
}

isRootlessDocker() {
    docker info --format '{{.SecurityOptions}}' 2>/dev/null | grep -q rootless
}

runContainerE2eTestsIfAvailable() {
    if ! docker info &> /dev/null; then
        echo "=== SKIP container e2e tests: docker not available ==="
        return
    fi

    if isRootlessDocker; then
        echo "=== SKIP container e2e tests: rootless Docker (UID remapping breaks bind mount writes) ==="
        return
    fi

    echo "=== Running container e2e-gradlew ==="
    "$SCRIPT_DIR/test-end-to-end-with-gradlew.sh" --force container

    echo "=== Running container e2e-pytest ==="
    "$SCRIPT_DIR/test-end-to-end-with-pytest.sh" --force container
}

runNonoE2eTestsIfAvailable
runContainerE2eTestsIfAvailable
