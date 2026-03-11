# Isolarium container environment
# Provides an isolated development environment for autonomous coding agents

# Base image (most stable)
FROM ubuntu:24.04

# System dependencies (stable - rarely changes)
RUN apt-get update && apt-get install -y \
    git \
    curl \
    wget \
    ca-certificates \
    gnupg \
    unzip \
    zip \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# Node.js LTS (stable - changes with LTS releases)
RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# GitHub CLI (stable - changes with releases)
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
        | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

# Non-root user (stable) - remove ubuntu user that claims UID 1000 in Ubuntu 24.04
# HOST_UID ensures container user matches host UID for bind mount write access
ARG HOST_UID=1000
RUN userdel -r ubuntu 2>/dev/null || true \
    && useradd -m -s /bin/bash -u $HOST_UID isolarium \
    && echo "isolarium ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/isolarium

# Git worktree support: create symlinks from host paths to container mount points
# Only chmod directories we create (not pre-existing system dirs like /tmp)
ARG WORKTREE_HOST_PATH=""
ARG MAIN_REPO_HOST_PATH=""
RUN if [ -n "$WORKTREE_HOST_PATH" ] && [ -n "$MAIN_REPO_HOST_PATH" ]; then \
        chmod_new_parents() { \
            dir="$(dirname "$1")"; \
            while [ "$dir" != "/" ] && [ ! -d "$dir" ]; do dir="$(dirname "$dir")"; done; \
            top="$dir"; \
            mkdir -p "$(dirname "$1")"; \
            dir="$(dirname "$1")"; \
            while [ "$dir" != "/" ] && [ "$dir" != "$top" ]; do chmod 555 "$dir"; dir="$(dirname "$dir")"; done; \
        }; \
        chmod_new_parents "$WORKTREE_HOST_PATH" \
        && ln -s /home/isolarium/repo "$WORKTREE_HOST_PATH" \
        && chmod_new_parents "$MAIN_REPO_HOST_PATH" \
        && ln -s /home/isolarium/main-repo "$MAIN_REPO_HOST_PATH"; \
    fi

# Switch to non-root user for remaining installs
USER isolarium
WORKDIR /home/isolarium

# Claude Code via npm (user-level tool)
RUN sudo npm install -g @anthropic-ai/claude-code

# uv (Python package manager)
RUN curl -LsSf https://astral.sh/uv/install.sh | sh

# SDKMAN, Java 17, Gradle 8.14 (user-level tools)
RUN curl -s "https://get.sdkman.io" | bash
RUN bash -c '\
    source "$HOME/.sdkman/bin/sdkman-init.sh" \
    && echo "sdkman_auto_answer=true" > "$HOME/.sdkman/etc/config" \
    && echo "sdkman_selfupdate_feature=false" >> "$HOME/.sdkman/etc/config" \
    && sdk install java 17.0.13-tem \
    && sdk install gradle 8.14'

# Configure git for container environment
RUN git config --global credential.https://github.com.helper '!gh auth git-credential' \
    && git config --global --add safe.directory '*'

# Working directory for mounted project
RUN mkdir -p /home/isolarium/repo /home/isolarium/main-repo
WORKDIR /home/isolarium/repo

# Keep container running
CMD ["sleep", "infinity"]
