#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ISOLARIUM="${SCRIPT_DIR}/../bin/isolarium"
VM_NAME="${1:-nono-gradle}"

"${ISOLARIUM}" --name "${VM_NAME}" shell <<'REMOTE'
set -euo pipefail

NONO_VERSION="0.12.0"
GO_VERSION="1.23.6"
export PATH=$PATH:/usr/local/go/bin

if command -v nono &>/dev/null && [[ "$(nono --version)" == *"${NONO_VERSION}"* ]]; then
  echo "nono ${NONO_VERSION} already installed."
else
  echo "Installing nono ${NONO_VERSION} from source..."

  if ! dpkg -s build-essential &>/dev/null; then
    echo "Installing build-essential..."
    sudo apt-get update -qq
    sudo apt-get install -y -qq build-essential pkg-config libdbus-1-dev
  fi

  if ! command -v cargo &>/dev/null; then
    echo "Installing Rust..."
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
    source "$HOME/.cargo/env"
  fi

  git clone --branch "v${NONO_VERSION}" --depth 1 https://github.com/always-further/nono.git /tmp/nono-build
  cd /tmp/nono-build
  cargo build --release
  sudo mv target/release/nono /usr/local/bin/
  cd /
  rm -rf /tmp/nono-build

  nono --version
  echo "nono installed successfully."
fi

if command -v go &>/dev/null && [[ "$(go version)" == *"go${GO_VERSION}"* ]]; then
  echo "Go ${GO_VERSION} already installed."
else
  ARCH=$(dpkg --print-architecture)
  echo "Installing Go ${GO_VERSION} for ${ARCH}..."

  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" -o /tmp/go.tar.gz
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf /tmp/go.tar.gz
  rm /tmp/go.tar.gz

  echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/golang.sh > /dev/null

  go version
  echo "Go installed successfully."
fi

mkdir -p "$HOME/.gradle"
REMOTE
