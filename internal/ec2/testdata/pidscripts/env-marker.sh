#!/usr/bin/env bash
set -euo pipefail

recordTheEnvironmentThisScriptRanWith() {
    printf 'ISOLARIUM_NAME=%s ISOLARIUM_TYPE=%s\n' "$ISOLARIUM_NAME" "$ISOLARIUM_TYPE" > "$1"
}

recordTheEnvironmentThisScriptRanWith "$HOME/repo/env-ran"
