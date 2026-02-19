# Support Nono Sandbox — Specification

## Purpose and Background

Isolarium is a CLI tool that provides secure, isolated execution environments for autonomous coding agents (such as Claude Code) on developer machines. It currently supports two isolation backends:

- **Lima VM**: Full virtual machine with strong kernel-level isolation. Slow to provision, macOS-only, agent works on a cloned repo inside the VM.
- **Docker container**: Persistent container with host directory mounted. Faster than VM, cross-platform, agent edits local files. Provides environment isolation but not security isolation.

Both backends create a **separate environment** (distinct filesystem, installed tools, injected credentials). This feature adds **nono** as a third isolation backend. Nono is fundamentally different: it provides **process-level capability-based sandboxing on the host OS**. Instead of creating a separate environment, nono wraps commands with fine-grained filesystem, network, and command restrictions. The agent runs directly on the host using the developer's existing tools, credentials, and project files — but with strict limits on what it can access.

Nono ("the opposite of YOLO") is purpose-built for sandboxing AI agents. Its macOS sandbox integration restricts filesystem access to explicitly allowed paths, blocks dangerous commands by default, and preserves TTY for interactive applications like Claude Code.

## Target Users and Personas

**Developer (primary)**: A software engineer who runs Claude Code against a local Python project and wants lightweight sandboxing without the overhead of a VM or container. They already have Python tooling (uv, pytest), Claude Code, and git installed on their machine. They want the agent restricted to their project directory and essential configuration files.

## Problem Statement and Goals

### Problem

Both existing backends require provisioning a separate environment (VM boot or container build), installing tools inside that environment, and managing credential transfer. For developers who already have the right tools installed on their host machine, this overhead is unnecessary. They want sandboxing — not environment provisioning.

### Goals

1. Provide a nono-based isolation mode that wraps commands with capability-based sandboxing, using the developer's existing host tools and credentials.
2. Implement nono as a third Backend interface implementation, following established patterns from the VM and container backends.
3. Ship with a hardcoded permission set targeting Claude Code + Python development as the initial use case.
4. Maintain backward compatibility — existing VM and container workflows are unaffected.

## In-Scope

- New `--type nono` option for `create`, `run`, `shell`, `destroy`, `status` commands
- `NonoBackend` struct implementing the `Backend` interface in `internal/backend/`
- New `internal/nono/` package for nono-specific operations (command building, metadata, state)
- Hardcoded permission set for Claude Code + Python (derived from nono-playground examples)
- Metadata storage at `~/.isolarium/{name}/nono/metadata.json`
- `create` validates nono is installed and records metadata
- `run` wraps commands with `nono run` plus permission flags
- `shell` wraps with `nono shell` plus permission flags
- `destroy` removes metadata
- `status` reports nono environments with state "configured"
- Mapping of isolarium's `--interactive` / `-i` flag to nono's `--exec` flag
- Validation that `--copy-session` and `--fresh-login` are rejected for nono
- Validation that VM-specific commands (`clone-repo`, `install-tools`, `install-workflow-tools-from-source`) are rejected for nono
- Adding `"nono"` to `knownEnvironmentTypes` and `knownTypes` for auto-detection and status listing
- Default environment name `isolarium-nono`

## Out-of-Scope

- Nono profile generation (deferred; all options passed on command line for now)
- User-configurable permission sets or `--allow`/`--read`/`--write` pass-through flags
- Java/Gradle permission set (only Claude Code + Python for initial implementation)
- Network blocking (`--net-block`)
- Command blocklist customization (`--allow-command`, `--block-command`)
- Credential copying or token minting (nono reads host credentials directly)
- `--work-directory` flag for nono (uses CWD)
- Automated nono installation
- Linux support (nono uses macOS sandbox; macOS-only for now)

## High-Level Functional Requirements

### FR1: Create a nono environment

`isolarium create --type nono [--name <name>]`

- Verify nono is installed by running `nono --version`. Error with a helpful message if nono is not found.
- Record the current working directory (absolute path) as the work directory.
- Write metadata to `~/.isolarium/{name}/nono/metadata.json` recording the type (`"nono"`), work directory, and creation timestamp.
- Default name: `isolarium-nono`.
- Reject `--work-directory` flag (error: `--work-directory is not supported with --type nono`).

### FR2: Run a command in the nono sandbox

`isolarium run [--interactive] [--name <name>] [--type nono] -- command [args...]`

- Resolve the environment by name and type (same auto-detection as VM/container).
- Reject `--copy-session` and `--fresh-login` flags with error: `--copy-session is not supported with --type nono` / `--fresh-login is not supported with --type nono`.
- Build a `nono run` command with the hardcoded permission set (see FR7).
- If `--interactive` / `-i` is set, include `--exec` in the nono flags for TTY preservation.
- Append `-- <command> [args...]` to the nono command.
- Execute the composed command on the host. Stream stdout/stderr. Propagate exit codes.

### FR3: Open an interactive shell in the nono sandbox

`isolarium shell [--name <name>] [--type nono]`

- Resolve the environment by name and type.
- Reject `--copy-session` flag with an error.
- Build a `nono shell` command with the hardcoded permission set (see FR7).
- Execute the composed command on the host.

### FR4: Destroy a nono environment

`isolarium destroy [--name <name>] [--type nono]`

- Resolve the environment by name and type.
- Remove metadata directory `~/.isolarium/{name}/nono/`.
- Print confirmation message.

### FR5: Show nono environment status

`isolarium status [--name <name>] [--type nono]`

- Nono environments appear in the status listing alongside VM and container environments.
- State is always `"configured"` (no running/stopped concept).
- Details column shows the work directory.

### FR6: Reject inapplicable commands

The following commands must error when invoked with `--type nono`:
- `clone-repo`: `clone-repo is not supported with --type nono`
- `install-tools`: `install-tools is not supported with --type nono`
- `install-workflow-tools-from-source`: `install-workflow-tools-from-source is not supported with --type nono`

### FR7: Hardcoded permission set

The nono backend builds commands with the following permission flags, targeting Claude Code + Python development:

**Project directory access:**
- `--allow .` — read/write access to the current working directory (the project)

**Claude Code credentials:**
- `--allow ~/.claude/` — Claude config directory
- `--allow-file ~/.claude.json` — Claude settings file
- `--allow ~/.claude.json.lock` — Claude lock file
- `--allow ~/.claude.json.tmp'.*'` — Claude temp files (atomic writes)
- `--read-file ~/Library/Keychains/login.keychain-db` — macOS keychain (read-only, for Claude auth)

**Python/uv tooling:**
- `--allow ~/.cache/uv` — uv package cache (read/write for installs)
- `--read ~/.local/share/uv` — uv data directory (read-only)

**Network:** Allowed (nono's default; no `--net-block` flag).

**Command blocklist:** Nono's built-in defaults (blocks `rm`, `dd`, `chmod`, etc.).

### FR8: Backend interface implementation

`NonoBackend` implements all seven `Backend` interface methods:

| Method | Behavior |
|--------|----------|
| `Create(name, opts)` | Validate nono installed, write metadata |
| `Destroy(name)` | Remove metadata directory |
| `Exec(name, envVars, args)` | Build and execute `nono run` with permissions + args |
| `ExecInteractive(name, envVars, args)` | Build and execute `nono run --exec` with permissions + args |
| `OpenShell(name, envVars)` | Build and execute `nono shell` with permissions |
| `GetState(name)` | Return `"configured"` |
| `CopyCredentials(name, credentials)` | No-op (return nil) |

### FR9: Environment identity and auto-detection

- Nono environments use the existing `(name, type)` identity pattern.
- Metadata stored at `~/.isolarium/{name}/nono/`.
- Add `"nono"` to `knownEnvironmentTypes` in `resolve_env.go` and `knownTypes` in `environment.go`.
- Auto-detection works the same as VM/container: if only one type exists for a name, use it; if multiple exist, require `--type`.

### FR10: Default name

- `--name` defaults to `isolarium-nono` when `--type nono` is specified and `--name` is not explicitly set.
- Follows the established pattern (`isolarium` for VM, `isolarium-container` for container).

### FR11: Metadata format

`~/.isolarium/{name}/nono/metadata.json`:

```json
{
  "type": "nono",
  "work_directory": "/absolute/path/to/project",
  "created_at": "2026-02-19T10:42:00Z"
}
```

## Security Requirements

### Nono security model

Nono provides **capability-based process sandboxing** using macOS sandbox profiles. The agent process runs on the host but can only access explicitly allowed filesystem paths. This is stronger than no sandboxing but weaker than VM isolation (shared kernel, shared network, host-level process).

### Permission boundaries

| Resource | Access | Rationale |
|----------|--------|-----------|
| Project directory (CWD) | Read/write | Agent must edit project files |
| `~/.claude/` | Read/write | Claude Code configuration and session |
| `~/.claude.json` | Read/write | Claude settings |
| macOS Keychain | Read-only | Claude authentication |
| `~/.cache/uv` | Read/write | Python package cache |
| `~/.local/share/uv` | Read-only | uv installation data |
| All other paths | Denied | Sandbox default |
| Network | Allowed | Claude API, package downloads, git |
| Destructive commands (`rm`, `dd`, etc.) | Blocked | Nono's built-in safety defaults |

### Credential security

| Operation | Who | Authorization | Constraints |
|-----------|-----|--------------|-------------|
| `create` | Developer on host | nono must be installed | Records metadata only |
| `run` / `shell` | Developer on host | None (uses host credentials directly) | Sandbox restricts which credentials are accessible |
| `destroy` | Developer on host | None | Only removes isolarium metadata |

### Comparison with other backends

| Property | VM | Container | Nono |
|----------|-----|-----------|------|
| Kernel isolation | Yes (separate kernel) | No (shared kernel) | No (shared kernel) |
| Filesystem isolation | Yes (no host mounts) | Partial (one mount) | Capability-based (allowed paths only) |
| Tool environment | Provisioned inside VM | Provisioned inside container | Uses host tools directly |
| Credential handling | Copied/minted | Copied/injected | Read from host (sandbox-gated) |
| Command restrictions | None | None | Nono's built-in blocklist |
| Provisioning time | Minutes | Seconds | Instant (metadata only) |

## Non-Functional Requirements

### Performance

- `create --type nono` should complete in under 1 second (validates nono, writes metadata).
- `run` and `shell` commands should add negligible overhead beyond nono's own sandbox setup time.

### Portability

- macOS only (nono uses the macOS sandbox). Linux support is out of scope.

### Reliability

- If nono is not installed, `create --type nono` must fail with a clear error message: `nono is not installed. Install nono to use sandbox mode.`
- If the nono command fails (e.g., sandbox setup error), the exit code and stderr must be propagated to the user.

### Backward Compatibility

- All existing VM and container commands and workflows continue to work unchanged.
- The `--type` flag defaults to `vm` (unchanged).
- The `--name` flag default for VM mode remains `isolarium` (unchanged).
- Status listing includes nono alongside VM and container environments.

## Success Metrics

1. A developer can go from `isolarium create --type nono` to `isolarium run -i --type nono -- claude` in under 2 seconds.
2. All supported commands (`create`, `run`, `shell`, `destroy`, `status`) work correctly for nono mode.
3. Claude Code running under nono can read/write project files, access its own config, and reach the Anthropic API.
4. Claude Code running under nono cannot access filesystem paths outside the allowed set.
5. VM, container, and nono environments can coexist without interference.

## Epics and User Stories

### Epic 1: Nono Backend Implementation

Implement the `NonoBackend` struct and `internal/nono/` package with command building and metadata operations.

- **US1.1**: As a developer, I want `NonoBackend` to implement the `Backend` interface so that the existing CLI routing works for nono without changing command implementations.
- **US1.2**: As a developer, I want a `nono` package that builds `nono run` and `nono shell` command lines with the correct permission flags so that the sandbox is configured consistently.

### Epic 2: Backend Resolution and Environment Identity

Wire nono into the backend resolution, auto-detection, and default naming systems.

- **US2.1**: As a user, I want `--type nono` to resolve to the `NonoBackend` so that I can select nono as my isolation mode.
- **US2.2**: As a user, I want auto-detection to recognize nono environments by scanning for `~/.isolarium/{name}/nono/` directories so that I don't have to pass `--type` on every command after creation.
- **US2.3**: As a user, I want the default name to be `isolarium-nono` when I specify `--type nono` without `--name`.

### Epic 3: CLI Command Integration

Update CLI commands to route nono through the Backend interface and validate flag compatibility.

- **US3.1**: As a user, I want `isolarium create --type nono` to validate nono is installed and record metadata so that subsequent commands know the environment exists.
- **US3.2**: As a user, I want `isolarium run --type nono -- <command>` to wrap my command with nono sandboxing so that the command runs with restricted permissions.
- **US3.3**: As a user, I want `isolarium run -i --type nono -- claude` to pass `--exec` to nono so that Claude Code's interactive UI works correctly.
- **US3.4**: As a user, I want `isolarium shell --type nono` to open a sandboxed interactive shell in my project directory.
- **US3.5**: As a user, I want `isolarium destroy --type nono` to remove the nono environment metadata.
- **US3.6**: As a user, I want `isolarium status` to list nono environments alongside VMs and containers with state "configured" and the work directory.
- **US3.7**: As a user, I want clear error messages when I use inapplicable flags (`--copy-session`, `--fresh-login`, `--work-directory`) or VM-only commands (`clone-repo`, `install-tools`) with `--type nono`.

## User-Facing Scenarios

### Scenario 1: Create and run Claude Code in a nono sandbox (primary)

1. Developer navigates to their Python project directory (a git repo or worktree).
2. Developer runs `isolarium create --type nono`.
3. Isolarium verifies nono is installed and writes metadata to `~/.isolarium/isolarium-nono/nono/metadata.json`.
4. Developer runs `isolarium run -i --type nono -- claude`.
5. Isolarium builds the nono command: `nono run --allow . --allow ~/.claude/ --allow-file ~/.claude.json --allow ~/.claude.json.lock --allow '~/.claude.json.tmp.*' --read-file ~/Library/Keychains/login.keychain-db --allow ~/.cache/uv --read ~/.local/share/uv --exec -- claude`
6. Claude Code starts with full interactive UI, can read/write project files, access its config, and reach the Anthropic API.
7. Claude Code cannot access paths outside the allowed set (e.g., `~/.ssh/`, `~/Documents/`, other projects).
8. Developer exits Claude Code. Exit code is propagated.

### Scenario 2: Run a non-interactive command in the sandbox

1. Developer has an existing nono environment (from scenario 1).
2. Developer runs `isolarium run --type nono -- uv run --python 3.12 --with pytest python3 -m pytest tests/ -v`.
3. Isolarium wraps the command with `nono run` (without `--exec` since `-i` was not specified).
4. Test output streams to the terminal. Exit code is propagated.

### Scenario 3: Drop into a sandboxed shell

1. Developer has an existing nono environment.
2. Developer runs `isolarium shell --type nono`.
3. Isolarium executes `nono shell` with the permission set.
4. Developer gets an interactive bash shell restricted to the allowed paths.
5. Developer can run `git status`, `uv run pytest`, etc. within the sandbox.

### Scenario 4: Status listing with mixed environment types

1. Developer has a VM (`isolarium`), a container (`isolarium-container`), and a nono environment (`isolarium-nono`).
2. Developer runs `isolarium status`.
3. Output shows all three:
   ```
   NAME                  TYPE        STATE       DETAILS
   isolarium             vm          running     myorg/myrepo (main)
   isolarium-container   container   running     /Users/dev/src/myrepo
   isolarium-nono        nono        configured  /Users/dev/src/myrepo
   ```

### Scenario 5: Inapplicable flag rejection

1. Developer runs `isolarium run --copy-session --type nono -- claude`.
2. Isolarium errors: `--copy-session is not supported with --type nono`.
3. Developer runs `isolarium run --type nono -- claude` (without the flag) — succeeds.

### Scenario 6: Nono not installed

1. Developer runs `isolarium create --type nono` on a machine without nono.
2. Isolarium runs `nono --version`, detects nono is not available, and errors: `nono is not installed. Install nono to use sandbox mode.`

### Scenario 7: Destroy a nono environment

1. Developer runs `isolarium destroy --type nono`.
2. Isolarium removes `~/.isolarium/isolarium-nono/nono/`.
3. Developer runs `isolarium status` — nono environment is no longer listed.
