#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

# pre-commit sets GIT_INDEX_FILE during stash, which breaks git worktree tests
unset GIT_INDEX_FILE

echo "=== Running Go tests ==="

go test ./...
