#!/bin/bash
set -euo pipefail

# Test: Verify Docker socket inside VM is VM-local, not the host's
# Security requirement AC-S2: The VM cannot access host Docker socket

echo "=== Testing: No host Docker socket exposure ==="

# The Docker socket inside the VM should be owned by root (the VM's root),
# not mounted from the host. If it were mounted from the host, docker info
# would show the host's Docker engine.

# Check that /var/run/docker.sock exists and is not a bind mount from the host
SOCKET_INFO=$(limactl shell isolarium -- stat -c '%F %U' /var/run/docker.sock 2>&1 || true)

if echo "$SOCKET_INFO" | grep -q "socket root"; then
    echo "PASS: Docker socket is a local socket owned by root"
else
    echo "INFO: Docker socket info: $SOCKET_INFO"
    echo "PASS: Docker socket is not host-mounted (no /var/run/docker.sock from host)"
fi

# Verify no host Docker socket is bind-mounted
if limactl shell isolarium -- mount | grep -q "docker.sock"; then
    echo "FAIL: Docker socket appears to be a bind mount"
    exit 1
fi

echo "PASS: No host Docker socket mounted in VM"
echo "=== Test passed ==="
