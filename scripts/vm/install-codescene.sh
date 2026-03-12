#!/bin/bash
set -euo pipefail

curl -fsSL https://downloads.codescene.io/enterprise/cli/install-cs-tool.sh -o /tmp/install-cs.sh
echo | sh /tmp/install-cs.sh
rm -f /tmp/install-cs.sh

export PATH=$PATH:$HOME/.local/bin
sudo ln -sf "$HOME/.local/bin/cs" /usr/local/bin/cs

cs --version
