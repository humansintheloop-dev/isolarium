#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ISOLARIUM="${SCRIPT_DIR}/../bin/isolarium"
PROJECT_ROOT="${SCRIPT_DIR}/.."
VM_NAME="${VM_NAME:-nono-gradle}"
VM_REPO="${VM_REPO:-$("${ISOLARIUM}" --name "${VM_NAME}" shell <<< "echo \$HOME/repo")}"

if [ $# -eq 0 ]; then
  echo "Usage: $0 <file1> [file2] ..."
  echo "Paths are relative to project root."
  exit 1
fi

for file in "$@"; do
  src="${PROJECT_ROOT}/${file}"
  if [ ! -f "$src" ]; then
    echo "File not found: $file"
    exit 1
  fi
  dest="${VM_REPO}/${file}"
  echo "Copying ${file}..."
  "${ISOLARIUM}" --name "${VM_NAME}" shell <<REMOTE
mkdir -p "$(dirname "${dest}")"
cat > "${dest}" << 'FILECONTENTS'
$(cat "$src")
FILECONTENTS
REMOTE
done

echo "Done."
