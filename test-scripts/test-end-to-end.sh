#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SKIP_DOCKER_INTEGRATION=false

for arg in "$@"; do
    case "$arg" in
        --skip-docker-integration) SKIP_DOCKER_INTEGRATION=true ;;
        *)
            echo "Usage: $0 [--skip-docker-integration]"
            exit 1
            ;;
    esac
done

echo "=== Cleaning up existing VM ==="

"$SCRIPT_DIR/clean.sh"

echo ""
"$SCRIPT_DIR/test-unit.sh"

if [ "$SKIP_DOCKER_INTEGRATION" = false ]; then
    echo ""
    "$SCRIPT_DIR/test-docker-integration.sh"

    echo ""
    "$SCRIPT_DIR/test-container-isolation-scripts.sh"
fi

echo ""
"$SCRIPT_DIR/test-lima-integration.sh"

echo ""
"$SCRIPT_DIR/test-claude-integration.sh"

echo ""
"$SCRIPT_DIR/test-nono-integration.sh"

echo ""
"$SCRIPT_DIR/test-security.sh"

echo ""
"$SCRIPT_DIR/test-cleanup.sh"

echo ""
echo "=== All tests passed ==="
