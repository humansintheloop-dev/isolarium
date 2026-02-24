#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== Cleaning up existing VM ==="

"$SCRIPT_DIR/clean.sh"

echo ""
"$SCRIPT_DIR/test-unit.sh"

echo ""
"$SCRIPT_DIR/test-docker-integration.sh"

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
