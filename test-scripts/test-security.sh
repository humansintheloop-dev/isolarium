#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== Running security verification tests ==="

if command -v limactl &> /dev/null; then
    if limactl list --json | grep -q '"name":"isolarium"'; then
        "$SCRIPT_DIR/test-no-host-mounts.sh"
        "$SCRIPT_DIR/test-no-docker-socket.sh"
        "$SCRIPT_DIR/test-no-git-credentials.sh"
    else
        echo "SKIP: No isolarium VM exists, skipping security tests"
    fi
else
    echo "SKIP: Lima not installed, skipping security tests"
fi
