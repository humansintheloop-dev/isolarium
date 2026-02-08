#!/bin/bash
set -euo pipefail

# Test: Verify no ambient git credentials exist in the VM
# Security requirement AC-S5: No personal git credentials are present in the VM

echo "=== Testing: No ambient git credentials ==="

# Check 1: No global credential helper configured
CRED_HELPER=$(limactl shell isolarium -- git config --global credential.helper 2>&1 || true)
if [ -z "$CRED_HELPER" ]; then
    echo "PASS: No global git credential helper configured"
else
    echo "FAIL: Global git credential helper found: $CRED_HELPER"
    exit 1
fi

# Check 2: No ~/.git-credentials file
if limactl shell isolarium -- test -f ~/.git-credentials 2>/dev/null; then
    echo "FAIL: ~/.git-credentials file exists in VM"
    exit 1
fi
echo "PASS: No ~/.git-credentials file in VM"

# Check 3: No ~/.gitconfig with credentials
GITCONFIG=$(limactl shell isolarium -- cat ~/.gitconfig 2>/dev/null || true)
if echo "$GITCONFIG" | grep -qi "credential"; then
    echo "FAIL: ~/.gitconfig contains credential configuration"
    exit 1
fi
echo "PASS: No credential entries in ~/.gitconfig"

echo "=== Test passed ==="
