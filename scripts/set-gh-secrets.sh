#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/../.env.local"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Error: .env.local not found at $ENV_FILE"
  exit 1
fi

# shellcheck source=/dev/null
source "$ENV_FILE"

CODESCENE_VARS=(CS_ACCESS_TOKEN CS_ACE_ACCESS_TOKEN)

for var in "${CODESCENE_VARS[@]}"; do
  value="${!var:-}"
  if [[ -z "$value" ]]; then
    echo "Warning: $var not found in .env.local, skipping"
    continue
  fi
  echo "$value" | gh secret set "$var"
  echo "Set secret: $var"
done
