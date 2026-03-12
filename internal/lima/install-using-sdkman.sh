#!/bin/bash
set -ex -o pipefail

# Set configuration for no interaction
export SDKMAN_DIR="$HOME/.sdkman"
# shellcheck source=/dev/null
[[ -s "$SDKMAN_DIR/bin/sdkman-init.sh" ]] && source "$SDKMAN_DIR/bin/sdkman-init.sh"

# Configure to auto-answer
echo "sdkman_auto_answer=true" > "$SDKMAN_DIR/etc/config"
echo "sdkman_selfupdate_feature=false" >> "$SDKMAN_DIR/etc/config"

# Install without interaction
sdk install java 17.0.13-tem
sdk install gradle 8.14

# Create symlinks for java in /usr/local/bin for non-interactive shell access
sudo ln -sf "$HOME/.sdkman/candidates/java/current/bin/java" /usr/local/bin/java
sudo ln -sf "$HOME/.sdkman/candidates/java/current/bin/javac" /usr/local/bin/javac

# Set JAVA_HOME system-wide for tools like Maven/Gradle
echo "JAVA_HOME=$HOME/.sdkman/candidates/java/current" | sudo tee -a /etc/environment
