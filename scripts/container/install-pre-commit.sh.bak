#!/bin/bash
set -euo pipefail

sudo apt-get update && sudo apt-get install -y python3 python3-pip python3-venv && sudo rm -rf /var/lib/apt/lists/*
pip3 install --user --break-system-packages pre-commit

export PATH=$PATH:$HOME/.local/bin
pre-commit --version
