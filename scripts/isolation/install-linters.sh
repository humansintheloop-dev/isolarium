#!/bin/bash
set -euo pipefail

export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin

curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "$(go env GOPATH)/bin"
sudo ln -sf "$(go env GOPATH)/bin/golangci-lint" /usr/local/bin/golangci-lint
sudo apt-get update && sudo apt-get install -y shellcheck && sudo rm -rf /var/lib/apt/lists/*

golangci-lint --version
shellcheck --version
