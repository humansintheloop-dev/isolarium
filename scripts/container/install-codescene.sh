#!/bin/bash
set -euo pipefail

mkdir -p "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"

curl -fsSL https://downloads.codescene.io/enterprise/cli/install-cs-tool.sh -o /tmp/install-cs.sh

sh /tmp/install-cs.sh < /dev/null
rm -f /tmp/install-cs.sh

cs --version
