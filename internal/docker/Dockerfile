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

# Non-root user (stable)
RUN useradd -m -s /bin/bash -u 1000 isolarium

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

# Configure git credential helper for GitHub (uses GH_TOKEN injected at runtime)
RUN git config --global credential.https://github.com.helper '!gh auth git-credential'

# Working directory for mounted project
RUN mkdir -p /home/isolarium/repo
WORKDIR /home/isolarium/repo

# Keep container running
CMD ["sleep", "infinity"]
