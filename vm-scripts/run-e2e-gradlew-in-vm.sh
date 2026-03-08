#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ISOLARIUM="${SCRIPT_DIR}/../bin/isolarium"
VM_NAME="${1:-nono-gradle}"

"${ISOLARIUM}" --name "${VM_NAME}" shell <<'REMOTE'
set -euo pipefail

export PATH=$PATH:/usr/local/go/bin

cd ~/repo
test-scripts/test-end-to-end-with-gradlew.sh --force
REMOTE
