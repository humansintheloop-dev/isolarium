#!/bin/bash
set -euo pipefail

# End-to-end test runner for isolarium
# Runs unit tests, integration tests, and cleanup in order

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=== Cleaning up existing VM ==="
"$SCRIPT_DIR/clean.sh"

echo ""
echo "=== Running unit tests ==="
go test ./...

echo ""
echo "=== Running Docker integration tests ==="
if docker info &> /dev/null; then
    go test -tags=integration ./internal/docker/...
else
    echo "SKIP: Docker not available, skipping Docker integration tests"
fi

echo ""
echo "=== Running Lima integration tests ==="
if command -v limactl &> /dev/null; then
    go test -tags=integration ./internal/lima/... ./internal/claude/...
else
    echo "SKIP: Lima not installed, skipping Lima integration tests"
fi

echo ""
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

echo ""
echo "=== Running cleanup ==="
if command -v limactl &> /dev/null; then
    go test -tags=cleanup -run TestDestroyCommand ./...
else
    echo "SKIP: Lima not installed, skipping cleanup"
fi

echo ""
echo "=== All tests passed ==="
