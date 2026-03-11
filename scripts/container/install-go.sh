#!/bin/bash
set -euo pipefail

GO_VERSION=1.22.12

curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
rm /tmp/go.tar.gz

# shellcheck disable=SC2016
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> "$HOME/.bashrc"
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin

go version
