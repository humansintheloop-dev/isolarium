#!/bin/bash
set -euo pipefail

# Test: Verify no host filesystem mounts exist in the VM
# Security requirement AC-S1: The VM cannot access host filesystem (no mounts)

echo "=== Testing: No host filesystem mounts ==="

# Check 1: Lima template has mounts: []
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEMPLATE="$PROJECT_ROOT/internal/lima/template.yaml"

if grep -q "^mounts: \[\]" "$TEMPLATE"; then
    echo "PASS: Lima template has mounts: []"
else
    echo "FAIL: Lima template does not have mounts: []"
    exit 1
fi

# Check 2: Verify no host paths are mounted inside the VM
# Common macOS host paths that should NOT appear
HOST_PATHS=("/Users" "/System" "/Volumes" "/tmp/lima")

for path in "${HOST_PATHS[@]}"; do
    if limactl shell isolarium -- mount | grep -q "$path"; then
        echo "FAIL: Host path $path is mounted in VM"
        exit 1
    fi
done

echo "PASS: No host filesystem paths mounted in VM"
echo "=== Test passed ==="
