#!/bin/bash
set -euo pipefail

export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin

downloadWithRetry() {
    curl -fsSL --retry 5 --retry-delay 2 --retry-connrefused --retry-all-errors "$1" -o "$2"
}

downloadWithRetry https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh /tmp/install-golangci-lint.sh
sh /tmp/install-golangci-lint.sh -b "$(go env GOPATH)/bin"
rm -f /tmp/install-golangci-lint.sh
sudo ln -sf "$(go env GOPATH)/bin/golangci-lint" /usr/local/bin/golangci-lint
sudo apt-get update && sudo apt-get install -y shellcheck && sudo rm -rf /var/lib/apt/lists/*

golangci-lint --version
shellcheck --version
