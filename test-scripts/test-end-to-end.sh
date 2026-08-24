#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SKIP_DOCKER_INTEGRATION=false
WITH_EC2=false

for arg in "$@"; do
    case "$arg" in
        --skip-docker-integration) SKIP_DOCKER_INTEGRATION=true ;;
        --with-ec2) WITH_EC2=true ;;
        *)
            echo "Usage: $0 [--skip-docker-integration] [--with-ec2]"
            exit 1
            ;;
    esac
done

echo "=== Cleaning up existing VM ==="

"$SCRIPT_DIR/clean.sh"

echo ""
"$SCRIPT_DIR/test-unit.sh"

echo ""
"$SCRIPT_DIR/test-ec2-preflight.sh"

if [ "$WITH_EC2" = true ]; then
    echo ""
    "$SCRIPT_DIR/test-ec2.sh"
fi

if [ "$SKIP_DOCKER_INTEGRATION" = false ]; then
    echo ""
    "$SCRIPT_DIR/test-docker-integration.sh"
fi

echo ""
"$SCRIPT_DIR/test-precommit-in-vm.sh"

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
