#!/bin/bash
set -euo pipefail

# Clean up isolarium VM if it exists

if command -v limactl &> /dev/null; then
    if limactl list --format '{{.Name}}' 2>/dev/null | grep -q '^isolarium$'; then
        echo "Stopping and deleting isolarium VM..."
        limactl stop isolarium 2>/dev/null || true
        limactl delete isolarium
        echo "VM deleted"
    else
        echo "No isolarium VM exists"
    fi
else
    echo "Lima not installed, nothing to clean"
fi
