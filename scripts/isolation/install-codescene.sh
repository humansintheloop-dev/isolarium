#!/bin/bash
set -euo pipefail

mkdir -p "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"

downloadWithRetry() {
    curl -fsSL --retry 5 --retry-delay 2 --retry-connrefused --retry-all-errors "$1" -o "$2"
}

downloadWithRetry https://downloads.codescene.io/enterprise/cli/install-cs-tool.sh /tmp/install-cs.sh

patchInstallerForNonInteractiveUse() {
    if [ ! -t 0 ]; then
        sed -i 's|/dev/tty|/dev/stdin|g' /tmp/install-cs.sh
    fi
}

patchInstallerForNonInteractiveUse
echo | sh /tmp/install-cs.sh
rm -f /tmp/install-cs.sh

# shellcheck disable=SC2016
echo 'export PATH=$PATH:$HOME/.local/bin' >> "$HOME/.bashrc"
sudo ln -sf "$HOME/.local/bin/cs" /usr/local/bin/cs

cs --version
