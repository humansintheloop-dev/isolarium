#!/bin/bash
set -ex -o pipefail

# Set configuration for no interaction
export SDKMAN_DIR="$HOME/.sdkman"
# shellcheck source=/dev/null
[[ -s "$SDKMAN_DIR/bin/sdkman-init.sh" ]] && source "$SDKMAN_DIR/bin/sdkman-init.sh"

# Configure to auto-answer (create etc dir if SDKMAN provisioning left it incomplete)
mkdir -p "$SDKMAN_DIR/etc"
echo "sdkman_auto_answer=true" > "$SDKMAN_DIR/etc/config"
echo "sdkman_selfupdate_feature=false" >> "$SDKMAN_DIR/etc/config"

# Retry a failed download; clear SDKMAN's tmp dir so a partial archive
# from the failed attempt is not reused
installWithRetry() {
    local attempt
    for attempt in 1 2 3 4 5; do
        sdk install "$@" && return 0
        echo "sdk install $* failed (attempt $attempt of 5); retrying in 10s" >&2
        rm -rf "${SDKMAN_DIR:?}/tmp/"*
        sleep 10
    done
    return 1
}

# Install without interaction
installWithRetry java 17.0.13-tem
installWithRetry gradle 8.14

# Create symlinks for java in /usr/local/bin for non-interactive shell access
sudo ln -sf "$HOME/.sdkman/candidates/java/current/bin/java" /usr/local/bin/java
sudo ln -sf "$HOME/.sdkman/candidates/java/current/bin/javac" /usr/local/bin/javac

# Set JAVA_HOME system-wide for tools like Maven/Gradle
echo "JAVA_HOME=$HOME/.sdkman/candidates/java/current" | sudo tee -a /etc/environment
