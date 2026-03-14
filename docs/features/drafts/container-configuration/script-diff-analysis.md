# Script Diff Analysis: container/ vs vm/

Analysis of scripts in `scripts/container/` vs `scripts/vm/` to identify shared logic and environment-specific differences.

## Summary

**No scripts are identical.** All four scripts share the same core logic but differ in environment-specific concerns:

- **VM scripts** add `sudo ln -sf` symlinks into `/usr/local/bin/` (Lima VMs need binaries on the default PATH)
- **Container scripts** handle Docker-specific issues (no `/dev/tty`, pre-configured `$HOME/.local/bin`)

## Per-Script Analysis

### install-go.sh — DIFFERS

**Shared**: Downloads and extracts Go tarball to `/usr/local/go`.

**VM-only** (3 lines added):
```bash
sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
sudo ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
```
VM needs symlinks so `go` is on the default PATH.

### install-linters.sh — DIFFERS

**Shared**: Installs golangci-lint via its install script.

**VM-only** (1 line added):
```bash
sudo ln -sf "$(go env GOPATH)/bin/golangci-lint" /usr/local/bin/golangci-lint
```

### install-pre-commit.sh — DIFFERS

**Shared**: Installs pre-commit via pip.

**VM-only** (1 line added):
```bash
sudo ln -sf "$HOME/.local/bin/pre-commit" /usr/local/bin/pre-commit
```

### install-codescene.sh — DIFFERS (most divergent)

**Shared**: Downloads and runs the CodeScene CLI installer.

**Container-only**:
```bash
mkdir -p "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"
# Docker RUN has no /dev/tty; the installer reads from it for interactive prompts
sed -i 's|/dev/tty|/dev/stdin|g' /tmp/install-cs.sh
```

**VM-only**:
```bash
export PATH=$PATH:$HOME/.local/bin
sudo ln -sf "$HOME/.local/bin/cs" /usr/local/bin/cs
```

## Pattern

| Script | Container-specific | VM-specific |
|---|---|---|
| install-go.sh | — | symlinks to `/usr/local/bin` |
| install-linters.sh | — | symlink to `/usr/local/bin` |
| install-pre-commit.sh | — | symlink to `/usr/local/bin` |
| install-codescene.sh | `mkdir`, PATH setup, `/dev/tty` → `/dev/stdin` patch | symlink to `/usr/local/bin` |

**Consolidation opportunity**: Extract shared core into `scripts/shared/`, with thin environment wrappers that source the shared script then apply environment-specific fixups (symlinks for VM, tty patch for container).
