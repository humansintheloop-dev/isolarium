# Support Container-Based Isolation — Specification

## Purpose and Background

Isolarium is a CLI tool that provides secure, isolated execution environments for autonomous coding agents (such as Claude Code) on developer machines. Today, isolarium exclusively uses Lima VMs for isolation, which provides strong kernel-level security (separate kernel, no host filesystem mounts, GitHub App identity). However, the VM-only approach has limitations:

- **Slow provisioning**: Creating a Lima VM takes several minutes due to OS image download, boot, and tool installation.
- **macOS-only**: Lima is a macOS-specific tool. Isolarium cannot run on Linux hosts or in CI pipelines.
- **No local editing**: The VM clones the repository internally with no host mounts, so developers cannot edit files on the host while an agent runs.

This feature adds **container-based isolation** as a second isolation strategy, giving developers a faster, cross-platform, and local-development-friendly alternative to VMs.

## Target Users and Personas

**Developer (primary)**: A software engineer who runs coding agents against their own repositories on their local machine. They want fast iteration, the ability to edit files on the host while the agent runs, and a consistent tool environment inside the container.

**CI/CD pipeline operator**: An engineer configuring automated pipelines on Linux runners. They need isolarium to work without Lima, using Docker which is universally available in CI environments.

## Problem Statement and Goals

### Problem

Developers currently must wait minutes for VM provisioning before an agent can start. On Linux, isolarium is unusable since Lima is macOS-only. Developers who want to edit files while an agent works must use workarounds outside isolarium.

### Goals

1. Provide a container-based isolation mode that starts in seconds.
2. Enable isolarium to run on Linux hosts and in CI pipelines via Docker.
3. Allow developers to mount a host directory into the container so they can edit files locally while the agent operates inside the container.
4. Apply best-effort security hardening to the container while clearly documenting that container isolation is weaker than VM isolation.
5. Maintain backward compatibility — existing VM workflows are unaffected.

## In-Scope

- New `--type container` flag for `create`, `run`, `shell`, `destroy`, `status` commands
- New `--work-directory` flag for `create --type container` (defaults to current working directory)
- Embedded Dockerfile in the Go binary (analogous to `internal/lima/template.yaml`)
- Container lifecycle management: build image, start persistent container, exec into it, destroy it
- Credential handling: extract `gh auth token` for Git/GitHub, copy Claude credentials from macOS Keychain
- Metadata storage expansion to support (name, type) identity pairs
- `status` command updated to list all environments (VMs and containers)
- Best-effort container hardening (`--cap-drop=ALL`, `--security-opt=no-new-privileges`, non-root user)
- `Backend` interface to abstract VM and container operations

## Out-of-Scope

- Project-local Dockerfile overrides (embedded Dockerfile only)
- Docker-in-Docker inside the container
- `--work-directory` for VM mode (VMs continue to clone the repo internally)
- `clone-repo` and `install-tools` commands for container mode
- Automated Docker installation
- gVisor or Kata Containers integration
- Container network policy configuration beyond Docker defaults
- Multi-architecture image builds (build for the host architecture only)

## High-Level Functional Requirements

### FR1: Create a container environment

`isolarium create --type container [--work-directory <path>] [--name <name>]`

- Verify Docker is available by running `docker info`. Error with a helpful message if Docker is not installed or not running.
- Build the Docker image from the embedded Dockerfile if an image with the expected tag does not already exist.
- Start a persistent container (`docker run -d ... sleep infinity`) with:
  - `--work-directory` (or cwd) bind-mounted as `~/repo` inside the container
  - Security flags: `--cap-drop=ALL`, `--security-opt=no-new-privileges`
  - Non-root user
  - Container named per the `--name` flag (default: `isolarium-container`)
- Write metadata to `~/.isolarium/{name}/container/metadata.json` recording the type, work directory (absolute path), and creation timestamp.

### FR2: Run a command in the container

`isolarium run [--copy-session] [--interactive] [--name <name>] [--type container] -- command [args...]`

- Resolve the environment by name. If only one type exists for that name, use it. If both VM and container exist, require `--type` to disambiguate.
- If `--copy-session` (default: true), read Claude credentials from the macOS Keychain and write them into the container at `~/.claude/.credentials.json` with permissions 600.
- Extract the GitHub token from the host by running `gh auth token`. Inject as `GH_TOKEN` environment variable via `docker exec -e`.
- Execute the command: `docker exec -e GH_TOKEN=<token> <container-name> <command> [args...]`
- For `--interactive`, use `docker exec -it` to attach a TTY.
- Stream stdout/stderr to the host terminal. Propagate exit codes.

### FR3: Open an interactive shell

`isolarium shell [--copy-session] [--name <name>] [--type container]`

- Same credential and token injection as `run`.
- Execute: `docker exec -it -e GH_TOKEN=<token> <container-name> bash`
- Working directory inside the container is `~/repo`.

### FR4: Destroy a container environment

`isolarium destroy [--name <name>] [--type container]`

- Resolve the environment by name/type (same disambiguation as `run`).
- Stop and remove the container: `docker rm -f <container-name>`.
- Remove metadata from `~/.isolarium/{name}/container/`.
- Do **not** remove the Docker image (it stays cached for fast re-creation).

### FR5: Show environment status

`isolarium status [--name <name>] [--type <type>]`

- With no flags: list **all** environments (VMs and containers).
- For each environment, display:
  - Name
  - Type (vm or container)
  - State (running, stopped, not created)
  - For VMs: repository (owner/repo), branch
  - For containers: work directory path
- `--name` and `--type` filter the output.

### FR6: Backend interface

- Define a `Backend` interface in a new package (e.g., `internal/backend/`) with methods corresponding to the operations above: `Create()`, `Destroy()`, `Exec()`, `ExecInteractive()`, `GetState()`, `CopyCredentials()`.
- Implement the interface in `internal/lima/` for VM operations (refactoring existing code).
- Implement the interface in `internal/docker/` for container operations (new code).
- CLI commands resolve the backend from the environment type and delegate all operations through the interface.

### FR7: Metadata and environment identity

- An environment is identified by the pair (name, type).
- Metadata is stored at `~/.isolarium/{name}/{type}/metadata.json`.
- The metadata file records: type, work directory (containers only), creation timestamp.
- When a command receives `--name` without `--type`:
  - Scan `~/.isolarium/{name}/` for subdirectories (`vm/`, `container/`).
  - If exactly one exists, use it.
  - If both exist, error with: `Multiple environments found for "{name}". Specify --type vm or --type container.`
  - If none exists, error with: `No environment found for "{name}".`

### FR8: Embedded Dockerfile

- Embed a Dockerfile in the Go binary using `//go:embed` (same pattern as `template.yaml` and `install-using-sdkman.sh`).
- Base image: Ubuntu 24.04 LTS.
- Create a non-root user for running the agent.
- Pre-installed tools (system-level): git, curl, wget, Node.js LTS, GitHub CLI, unzip, zip.
- Pre-installed tools (user-level): Claude Code (npm global), uv, SDKMAN, Java 17.0.13 (Temurin), Gradle 8.14.
- Configure `gh auth git-credential` as the git credential helper for github.com.
- Clone and install workflow tools (humansintheloop-dev-workflow-and-tools repo), including plugins and i2code CLI.
- Working directory set to `~/repo`.

### FR9: Default names

- VM mode: `--name` defaults to `isolarium` (unchanged).
- Container mode: `--name` defaults to `isolarium-container`.
- The `--name` value is used directly as the Docker container name (no transformation).

## Security Requirements

### Container mode security model

Container isolation is **weaker than VM isolation**. Containers share the host kernel, and `--work-directory` gives the agent direct read-write access to a host directory. Container mode provides **environment isolation** (consistent tools and dependencies) but **not security isolation**. For untrusted workloads, users must use VM mode.

### Hardening measures

| Measure | Implementation |
|---------|---------------|
| Non-root user | Dockerfile creates a dedicated user; container runs as that user |
| Drop all capabilities | `docker run --cap-drop=ALL` |
| No privilege escalation | `docker run --security-opt=no-new-privileges` |
| Scoped filesystem access | Only `--work-directory` is mounted; no access to rest of host filesystem |
| Short-lived credentials | `GH_TOKEN` is injected per-command, not persisted in the image |

### Credential security

| Operation | Who | Authorization | Constraints |
|-----------|-----|--------------|-------------|
| `create` | Developer on host | Docker must be running | Docker daemon access required |
| `run` / `shell` | Developer on host | `gh auth token` must succeed (user logged in to gh CLI) | Token injected per-command, not stored in image or container filesystem |
| `run --copy-session` | Developer on host | macOS Keychain access (may prompt for password) | Credentials written to container with 600 permissions |
| `destroy` | Developer on host | Docker must be running | Only removes the named container |

### Documentation requirement

The CLI help text and project documentation must clearly state:
> Container mode provides environment isolation but not security isolation. For untrusted agents or code, use VM mode (`--type vm`).

## Non-Functional Requirements

### Performance

- `create --type container` with a cached image should complete in under 10 seconds (container start only).
- `create --type container` with a fresh image build should complete significantly faster than VM creation.
- `run` and `shell` commands should add negligible overhead beyond `docker exec` latency.

### Portability

- Container mode must work on macOS (Docker Desktop) and Linux (Docker Engine).
- The embedded Dockerfile must produce a working image on both `amd64` and `arm64` architectures (single-arch build for the host).

### Reliability

- If Docker is not running, `create --type container` must fail with a clear error message, not hang.
- If the container has stopped (e.g., Docker Desktop restart), `run`/`shell` should detect this and either restart it or provide a clear error.

### Backward Compatibility

- All existing VM commands and workflows continue to work unchanged.
- The `--type` flag defaults to `vm`.
- The `--name` flag default for VM mode remains `isolarium`.
- Existing metadata at `~/.isolarium/{name}/repo.json` continues to work for VM environments during a migration period.

## Success Metrics

1. A developer can go from `isolarium create --type container` to `isolarium shell` (with a working agent environment) in under 30 seconds on first run, and under 10 seconds on subsequent runs.
2. All five supported commands (`create`, `run`, `shell`, `destroy`, `status`) work correctly for container mode.
3. The same isolarium binary works on both macOS and Linux for container mode.
4. VM and container environments can coexist without interference.

## Epics and User Stories

### Epic 1: Backend Interface Extraction

Refactor the existing codebase to introduce a `Backend` interface, moving Lima-specific code behind the interface so that container support can be added without modifying CLI command logic.

- **US1.1**: As a developer, I want the CLI commands to operate through a `Backend` interface so that adding new isolation types does not require changing command implementations.
- **US1.2**: As a developer, I want the Lima VM backend to implement the `Backend` interface so that existing VM functionality is preserved.

### Epic 2: Environment Identity and Metadata

Expand the metadata system to support (name, type) identity pairs, with auto-detection and disambiguation.

- **US2.1**: As a user, I want environments to be identified by (name, type) so that I can have both a VM and a container with the same logical name.
- **US2.2**: As a user, I want subsequent commands to auto-detect the environment type by name so that I don't have to pass `--type` on every command.
- **US2.3**: As a user, I want a clear error message when I have both a VM and container with the same name and don't specify `--type`.

### Epic 3: Container Lifecycle

Implement the Docker backend for creating, running, shelling into, and destroying container environments.

- **US3.1**: As a user, I want to run `isolarium create --type container` to build an image and start a persistent container with my project directory mounted.
- **US3.2**: As a user, I want `isolarium run --type container -- <command>` to execute a command inside the container with credentials injected.
- **US3.3**: As a user, I want `isolarium shell --type container` to open an interactive bash session inside the container.
- **US3.4**: As a user, I want `isolarium destroy --type container` to remove the container while keeping the image cached.

### Epic 4: Embedded Dockerfile

Create and embed a Dockerfile that provides the same toolset as the VM template (minus Docker-in-Docker).

- **US4.1**: As a developer, I want the default container image to include git, Node.js, Claude Code, GitHub CLI, uv, Java 17, Gradle 8.14, and workflow tools so that agents have the same capabilities as in a VM.
- **US4.2**: As a developer, I want the Dockerfile to configure `gh auth git-credential` as the git credential helper so that `GH_TOKEN` injection enables both git and gh operations.

### Epic 5: Credential Handling

Implement credential injection for container mode using the developer's existing `gh` authentication and Claude session from macOS Keychain.

- **US5.1**: As a user, I want `run` and `shell` to automatically extract my `gh auth token` and inject it into the container so that git push/pull and gh CLI work without manual setup.
- **US5.2**: As a user, I want `--copy-session` to copy my Claude credentials into the container so that the agent can authenticate with Claude.

### Epic 6: Status Enhancements

Update the `status` command to list all environments across both types.

- **US6.1**: As a user, I want `isolarium status` to show all my environments (VMs and containers) with their name, type, state, and relevant details.
- **US6.2**: As a user, I want to filter status output with `--name` and `--type`.

## User-Facing Scenarios

These scenarios define the primary end-to-end workflows and are suitable for defining a steel-thread plan in a subsequent step.

### Scenario 1: Create and use a container environment (primary)

1. Developer navigates to their project directory.
2. Developer runs `isolarium create --type container`.
3. Isolarium verifies Docker is available, builds the image (if needed), starts a persistent container with cwd mounted as `~/repo`.
4. Developer runs `isolarium shell`.
5. Isolarium extracts `gh auth token`, copies Claude credentials, and opens an interactive bash session inside the container.
6. Developer verifies: git, node, java, gradle, claude, gh are available; `~/repo` contains their project files; `git push` works; `gh pr create` works.
7. Developer exits the shell and runs `isolarium status` — sees the container listed as running.
8. Developer runs `isolarium destroy --type container` — container is removed, image remains.

### Scenario 2: Run a one-off command in a container

1. Developer has an existing container environment (from scenario 1).
2. Developer runs `isolarium run -- claude --print "explain this codebase"`.
3. Isolarium injects credentials and executes the command inside the container.
4. Output streams to the developer's terminal. Exit code is propagated.

### Scenario 3: Coexistence of VM and container

1. Developer has an existing VM environment (name: "isolarium").
2. Developer runs `isolarium create --type container` (name defaults to "isolarium-container").
3. Developer runs `isolarium status` — sees both environments listed.
4. Developer runs `isolarium shell --name isolarium` — opens shell in the VM.
5. Developer runs `isolarium shell --name isolarium-container` — opens shell in the container.
6. Developer destroys each independently.

### Scenario 4: Disambiguation when names collide

1. Developer creates a VM with `--name myenv`.
2. Developer creates a container with `--name myenv`.
3. Developer runs `isolarium shell --name myenv` — gets an error: "Multiple environments found for 'myenv'. Specify --type vm or --type container."
4. Developer runs `isolarium shell --name myenv --type container` — succeeds.

### Scenario 5: Docker not available

1. Developer runs `isolarium create --type container` on a machine without Docker.
2. Isolarium runs `docker info`, detects Docker is not available, and errors with: "Docker is not installed or not running. Install Docker Desktop (macOS) or Docker Engine (Linux) to use container mode."
