#!/bin/bash
set -euo pipefail

CODESCENE_VERSION=0.4.170

curl -fsSL "https://downloads.codescene.io/enterprise/cli/${CODESCENE_VERSION}/codescene-cli_${CODESCENE_VERSION}_linux_amd64.tar.gz" -o /tmp/codescene-cli.tar.gz
sudo tar -C /usr/local/bin -xzf /tmp/codescene-cli.tar.gz run-codescene.sh
rm /tmp/codescene-cli.tar.gz
sudo chmod +x /usr/local/bin/run-codescene.sh

run-codescene.sh --version
