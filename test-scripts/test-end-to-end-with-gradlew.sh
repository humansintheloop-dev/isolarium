#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

FORCE=false

for arg in "$@"; do
    case "$arg" in
        --force) FORCE=true ;;
        *)
            echo "Usage: $0 [--force]"
            exit 1
            ;;
    esac
done

GOTEST_FLAGS=(-v -tags=e2e_gradlew)
if [ "$FORCE" = true ]; then
    GOTEST_FLAGS+=(-count=1)
fi

BINARY="bin/isolarium"
go build -o "$BINARY" ./cmd/isolarium

echo "=== Running gradlew build in nono ==="
go test "${GOTEST_FLAGS[@]}" -run "TestGradlew.*InNono_EndToEnd" ./cmd/isolarium/...
