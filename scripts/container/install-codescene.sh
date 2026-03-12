#!/bin/bash
set -euo pipefail

mkdir -p "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"

curl -fsSL https://downloads.codescene.io/enterprise/cli/install-cs-tool.sh -o /tmp/install-cs.sh

# Docker RUN has no /dev/tty; the installer reads from it for interactive prompts
sed -i 's|/dev/tty|/dev/stdin|g' /tmp/install-cs.sh
echo | sh /tmp/install-cs.sh
rm -f /tmp/install-cs.sh

cs --version
