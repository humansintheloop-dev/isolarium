#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ISOLARIUM="${SCRIPT_DIR}/../bin/isolarium"
VM_NAME="${1:-nono-gradle}"

"${ISOLARIUM}" --name "${VM_NAME}" shell <<'REMOTE'
set -euo pipefail

JAVA_HOME="$HOME/.sdkman/candidates/java/current"
export PATH="$JAVA_HOME/bin:$PATH"

echo "=== Java location ==="
which java
ls -la "$(which java)"

echo "=== Without nono ==="
java -version

echo "=== With nono ==="
nono run --allow-cwd \
  --allow "$HOME/.sdkman" \
  --override-deny "$HOME/.sdkman" \
  --read /usr/lib/locale \
  --override-deny /usr/lib/locale \
  --read /lib/aarch64-linux-gnu \
  --override-deny /lib/aarch64-linux-gnu \
  --read /usr/lib/aarch64-linux-gnu \
  --override-deny /usr/lib/aarch64-linux-gnu \
  -v \
  -- java -version
REMOTE
