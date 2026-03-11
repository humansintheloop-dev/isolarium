#!/bin/bash
set -euo pipefail

curl -fsSL https://downloads.codescene.io/enterprise/cli/install-cs-tool.sh | sh

cs --version
