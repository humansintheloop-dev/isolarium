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

echo "=== Running e2e-gradlew ==="

"$SCRIPT_DIR/test-end-to-end-with-gradlew.sh" --force nono container

echo "=== Running e2e-pytest ==="

"$SCRIPT_DIR/test-end-to-end-with-pytest.sh" --force nono container
