Now I have a thorough understanding of the codebase. Let me generate the plan.

# Support Container-Based Isolation — Implementation Plan

## Idea Type

**C. Platform/infrastructure capability** — This adds a new isolation strategy (Docker containers) alongside the existing VM strategy, expanding the platform's capabilities without changing the core user-facing value proposition.

## Instructions for Coding Agent

- IMPORTANT: Use simple commands that you have permission to execute. Avoid complex commands that may fail due to permission issues.

### Required Skills

Use these skills by invoking them before the relevant action:

| Skill | When to Use |
|-------|-------------|
| `idea-to-code:plan-tracking` | ALWAYS - track task completion in the plan file |
| `idea-to-code:tdd` | When implementing code - write failing tests first |
| `idea-to-code:commit-guidelines` | Before creating any git commit |
| `idea-to-code:incremental-development` | When writing multiple similar files (tests, classes, configs) |
| `idea-to-code:testing-scripts-and-infrastructure` | When building shell scripts or test infrastructure |
| `idea-to-code:dockerfile-guidelines` | When creating or modifying Dockerfiles |
| `idea-to-code:file-organization` | When moving, renaming, or reorganizing files |
| `idea-to-code:debugging-ci-failures` | When investigating CI build failures |
| `idea-to-code:test-runner-java-gradle` | When running tests in Java/Gradle projects |

### TDD Requirements

- NEVER write production code without first writing a failing test
- Before using Write on any `.go` file outside of `*_test.go`, ask: "Do I have a failing test?" If not, write the test first
- When task direction changes mid-implementation, return to TDD PLANNING state and write a test first

### Verification Requirements

- Hard rule: NEVER git commit, git push, or open a PR unless you have successfully run the project's test command and it exits 0
- Hard rule: If running tests is blocked for any reason (including permissions), ALWAYS STOP immediately. Print the failing command, the exact error output, and the permission/path required
- Before committing, ALWAYS print a Verification section containing the exact test command (NOT an ad-hoc command - it must be a proper test command such as `./test-scripts/*.sh`, `./gradlew build`/`./gradlew check`, or `go test ./...`), its exit code, and the last 20 lines of output

## Overview

This plan implements container-based isolation for isolarium, a Go CLI tool that currently only supports Lima VMs. The implementation follows a steel-thread approach, starting with a Backend interface extraction, then building the Docker backend incrementally through end-to-end scenarios.

**Current architecture:**
- CLI commands in `internal/cli/` directly call `internal/lima/` package functions
- Metadata stored at `~/.isolarium/<name>/repo.json`
- Embedded templates via `//go:embed` (template.yaml, install-using-sdkman.sh)
- Unit tests use `command.Runner` interface for mocking; integration tests use `//go:build integration`
- CI runs `./test-scripts/test-end-to-end.sh` which executes `go test ./...`

**Key design decisions:**
- Introduce a `Backend` interface in `internal/backend/` to abstract VM and container operations
- Create `internal/docker/` package parallel to `internal/lima/`
- Expand metadata system to support `(name, type)` identity pairs at `~/.isolarium/<name>/<type>/metadata.json`
- Embed a Dockerfile in the binary using `//go:embed`, following the same pattern as `template.yaml`
- Container credentials: `gh auth token` for GitHub (injected per-command as `GH_TOKEN`), Claude credentials copied via `docker exec`
- The `ssh` command is renamed to `shell` in the CLI (per spec FR3), keeping backward compatibility

---

## Steel Thread 1: Backend Interface and Docker Container Lifecycle (Create + Destroy)

This thread proves the fundamental architecture: a Backend interface abstraction, Docker container creation with an embedded Dockerfile, and container destruction. It establishes the pattern all subsequent threads build on.

- [ ] **Task 1.1: Define Backend interface and resolve backend from environment type**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/backend/...`
  - Observable: A `Backend` interface exists with `Create()`, `Destroy()`, `Exec()`, `ExecInteractive()`, `GetState()`, `CopyCredentials()` methods. A `ResolveBackend()` function returns the correct backend based on environment type string ("vm" or "container").
  - Evidence: Unit tests in `internal/backend/` verify `ResolveBackend("vm")` returns a Lima backend and `ResolveBackend("container")` returns a Docker backend, and that `ResolveBackend("unknown")` returns an error.
  - Steps:
    - [ ] Create `internal/backend/backend.go` with the `Backend` interface defining: `Create(name string, opts CreateOptions) error`, `Destroy(name string) error`, `Exec(name string, envVars map[string]string, args []string) (int, error)`, `ExecInteractive(name string, envVars map[string]string, args []string) (int, error)`, `GetState(name string) string`, `CopyCredentials(name string, credentials string) error`. `CreateOptions` includes `WorkDirectory string`.
    - [ ] Create `internal/backend/resolve.go` with `ResolveBackend(envType string) (Backend, error)` that returns a `LimaBackend` for "vm" and a `DockerBackend` for "container"
    - [ ] Create stub `internal/backend/lima_backend.go` that wraps existing `internal/lima/` functions to implement the `Backend` interface (methods can delegate to existing lima package functions)
    - [ ] Create stub `internal/backend/docker_backend.go` with `DockerBackend` struct implementing `Backend` — all methods return `ErrNotImplemented` for now
    - [ ] Write unit tests in `internal/backend/resolve_test.go` verifying correct backend resolution and error for unknown type

- [ ] **Task 1.2: `isolarium create --type container` builds image and starts persistent container**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: `DockerBackend.Create()` verifies Docker is available, builds an image from an embedded Dockerfile, starts a persistent container with the work directory bind-mounted as `/home/isolarium/repo`, security flags (`--cap-drop=ALL`, `--security-opt=no-new-privileges`), running as non-root user, and writes metadata to `~/.isolarium/<name>/container/metadata.json`.
  - Evidence: Unit tests verify: (1) `BuildImageCommand()` produces correct `docker build` args, (2) `BuildRunCommand()` produces correct `docker run -d` args with security flags, volume mount, and non-root user, (3) `WriteDockerTempfile()` writes embedded Dockerfile content to a temp directory, (4) metadata is written with correct type and work directory.
  - Steps:
    - [ ] Create embedded Dockerfile at `internal/docker/Dockerfile` with Ubuntu 24.04 base, non-root `isolarium` user (UID 1000), system tools (git, curl, wget, Node.js LTS, GitHub CLI, unzip, zip), user-level tools (Claude Code via npm, uv, SDKMAN, Java 17, Gradle 8.14), `gh auth git-credential` configured as git credential helper, and working directory `/home/isolarium/repo`. Follow Dockerfile guidelines: order instructions from most stable to least stable.
    - [ ] Create `internal/docker/docker.go` with `//go:embed Dockerfile` and functions: `BuildImageCommand(tag string, contextDir string) []string`, `BuildRunCommand(name, workDir, imageTag string) []string`, `BuildCheckDockerCommand() []string`
    - [ ] Create `internal/docker/metadata.go` with `DockerMetadata` struct (Type, WorkDirectory, CreatedAt) and `MetadataStore` for `~/.isolarium/<name>/container/metadata.json`
    - [ ] Create `internal/docker/create.go` with `Create(name string, workDir string)` orchestrating: check Docker available → build image if needed → run container → write metadata
    - [ ] Update `internal/backend/docker_backend.go` to delegate `Create()` to the new docker package

- [ ] **Task 1.3: `isolarium destroy --type container` removes container and metadata**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: `Destroy()` runs `docker rm -f <name>`, removes metadata from `~/.isolarium/<name>/container/`, and does NOT remove the Docker image.
  - Evidence: Unit tests verify: (1) `BuildDestroyCommand()` produces correct `docker rm -f` args, (2) metadata cleanup removes the container metadata directory, (3) destroy succeeds even if container doesn't exist (idempotent).
  - Steps:
    - [ ] Create `internal/docker/destroy.go` with `Destroy(name string)` and `BuildDestroyCommand(name string) []string`
    - [ ] Add metadata `Cleanup()` method to docker metadata store
    - [ ] Update `internal/backend/docker_backend.go` to delegate `Destroy()`

- [ ] **Task 1.4: Wire `--type` flag into CLI `create` and `destroy` commands**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/cli/... ./internal/backend/...`
  - Observable: `isolarium create --type container` calls `DockerBackend.Create()`. `isolarium create` (no flag) calls `LimaBackend.Create()` (default is "vm"). `isolarium destroy --type container` calls `DockerBackend.Destroy()`. The `--work-directory` flag is accepted for container mode and defaults to cwd.
  - Evidence: Unit tests verify: (1) `--type` flag parsing and default to "vm", (2) `--work-directory` flag parsing and default to cwd, (3) `--work-directory` rejected when `--type vm`
  - Steps:
    - [ ] Add `--type` persistent flag to root command (default "vm", valid values "vm" and "container")
    - [ ] Add `--work-directory` flag to `create` command (default to cwd, container mode only)
    - [ ] Update `newCreateCmd()` to resolve backend via `ResolveBackend(typeFlag)` and call `backend.Create()`
    - [ ] Update `newDestroyCmd()` to resolve backend and call `backend.Destroy()`
    - [ ] Add default name logic: "isolarium" for vm, "isolarium-container" for container (when `--name` not explicitly set)

- [ ] **Task 1.5: Update CI to run `go test ./...` including new packages**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-end-to-end.sh`
  - Observable: CI passes with the new `internal/backend/` and `internal/docker/` packages included in `go test ./...`
  - Evidence: `./test-scripts/test-end-to-end.sh` exits 0 with all unit tests passing
  - Steps:
    - [ ] Verify `test-scripts/test-end-to-end.sh` already runs `go test ./...` which will pick up new packages
    - [ ] Run `go test ./...` locally and confirm all tests pass

---

## Steel Thread 2: Execute Command in Container (`run`)

This thread implements command execution inside a running container, including GitHub token injection via `docker exec -e`.

- [ ] **Task 2.1: `docker exec` command builder with environment variable injection**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: `BuildExecCommand()` produces correct `docker exec` args with `-e GH_TOKEN=<token>` and working directory. `BuildInteractiveExecCommand()` adds `-it` flags.
  - Evidence: Unit tests verify command construction with and without env vars, with and without interactive flags, verifying correct argument ordering.
  - Steps:
    - [ ] Create `internal/docker/exec.go` with `BuildExecCommand(name string, envVars map[string]string, args []string) []string` and `BuildInteractiveExecCommand(name string, envVars map[string]string, args []string) []string`
    - [ ] Implement `ExecCommand()` and `ExecInteractiveCommand()` that build and run the docker exec command, streaming stdout/stderr and propagating exit codes
    - [ ] Update `internal/backend/docker_backend.go` to delegate `Exec()` and `ExecInteractive()`

- [ ] **Task 2.2: Wire `run` command to use backend for container mode**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/cli/...`
  - Observable: `isolarium run --type container -- echo hello` resolves the Docker backend and executes via `docker exec`. GitHub token is extracted from `gh auth token` and injected as `GH_TOKEN` env var.
  - Evidence: Unit tests verify the run command routes to the correct backend based on `--type` flag, and constructs the correct environment variables.
  - Steps:
    - [ ] Refactor `newRunCmd()` to resolve backend from `--type` flag
    - [ ] Extract GitHub token extraction into a shared function (works for both VM and container modes — uses `gh auth token` for containers, GitHub App token for VMs)
    - [ ] For container mode, inject GH_TOKEN via `docker exec -e` rather than GitHub App token minting

- [ ] **Task 2.3: Container state detection for `run` command**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: `GetState(name)` returns "running", "stopped", or "none" by inspecting the Docker container. If the container is stopped, `run` provides a clear error message suggesting `isolarium create --type container`.
  - Evidence: Unit tests verify `BuildInspectCommand()` produces correct args and `ParseContainerState()` correctly parses `docker inspect` output for running, stopped (exited), and non-existent containers.
  - Steps:
    - [ ] Create `internal/docker/state.go` with `BuildInspectCommand(name string) []string` and `ParseContainerState(output string) string`
    - [ ] Implement `GetState(name string) string` that runs docker inspect and parses the result
    - [ ] Update `internal/backend/docker_backend.go` to delegate `GetState()`

---

## Steel Thread 3: Interactive Shell in Container (`shell`)

- [ ] **Task 3.1: `isolarium shell --type container` opens interactive bash in container**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/... ./internal/cli/...`
  - Observable: `isolarium shell --type container` runs `docker exec -it -e GH_TOKEN=<token> -w /home/isolarium/repo <name> bash`. The `shell` command (currently named `ssh`) is updated or a new `shell` command is added alongside `ssh`.
  - Evidence: Unit tests verify: (1) `BuildShellCommand()` produces correct `docker exec -it` args with env vars and working directory, (2) CLI routes `shell` command to the correct backend based on type.
  - Steps:
    - [ ] Create `internal/docker/shell.go` with `BuildShellCommand(name string, envVars map[string]string) []string` and `OpenShell(name string, envVars map[string]string) (int, error)`
    - [ ] Add `newShellCmd()` in `internal/cli/` that works for both VM and container modes (uses backend interface). Keep the existing `ssh` command as an alias for backward compatibility.
    - [ ] Wire credential injection: for container mode, extract `gh auth token` and inject as GH_TOKEN

---

## Steel Thread 4: Copy Claude Credentials to Container

- [ ] **Task 4.1: Copy Claude credentials into container via `docker exec`**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: `CopyCredentials()` creates `~/.claude/` directory in the container, writes credentials to `~/.claude/.credentials.json`, and sets permissions to 600 — all via `docker exec` commands.
  - Evidence: Unit tests verify: (1) `BuildCreateClaudeDirCommand()` produces correct `docker exec` mkdir args, (2) `BuildWriteCredentialsCommand()` produces correct args to write credentials via stdin, (3) `BuildChmodCredentialsCommand()` produces correct chmod args.
  - Steps:
    - [ ] Create `internal/docker/session.go` with `CopyClaudeCredentials(name, credentials string) error` and command builder functions: `BuildCreateClaudeDirCommand()`, `BuildWriteCredentialsCommand()`, `BuildChmodCredentialsCommand()`
    - [ ] Update `internal/backend/docker_backend.go` to delegate `CopyCredentials()`
    - [ ] Wire `--copy-session` flag in `run` and `shell` commands for container mode (read from Keychain, write into container)

---

## Steel Thread 5: Environment Identity, Metadata, and Auto-Detection

This thread expands the metadata system to support (name, type) identity pairs and auto-detection.

- [ ] **Task 5.1: Metadata system supports (name, type) identity pairs**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/backend/...`
  - Observable: Metadata is stored at `~/.isolarium/<name>/<type>/metadata.json`. When `--name` is provided without `--type`, the system scans for subdirectories (`vm/`, `container/`). If exactly one exists, it auto-detects. If both exist, it errors with "Multiple environments found". If none exists, it errors with "No environment found".
  - Evidence: Unit tests verify: (1) scanning logic finds single type, (2) scanning logic detects ambiguity and returns correct error, (3) scanning logic returns "not found" error when no environment exists.
  - Steps:
    - [ ] Create `internal/backend/resolve_env.go` with `ResolveEnvironmentType(baseDir, name string) (string, error)` that scans `~/.isolarium/<name>/` for `vm/` and `container/` subdirectories
    - [ ] Update CLI commands (`run`, `shell`, `destroy`) to use `ResolveEnvironmentType()` when `--type` is not explicitly provided
    - [ ] Ensure `create` for VM mode writes metadata to `~/.isolarium/<name>/vm/` path (migration from old `~/.isolarium/<name>/repo.json` path)

- [ ] **Task 5.2: VM metadata migration to new (name, type) path structure**
  - TaskType: REFACTOR
  - Entrypoint: `go test ./internal/lima/... ./internal/backend/...`
  - Observable: No behavior change — existing VM metadata at `~/.isolarium/<name>/repo.json` continues to work. New VM metadata is written to `~/.isolarium/<name>/vm/metadata.json`. Reading falls back to old path if new path doesn't exist.
  - Evidence: Existing unit tests in `internal/lima/metadata_test.go` continue to pass. New tests verify fallback reading from old path format.
  - Steps:
    - [ ] Update `internal/lima/metadata.go` to write to new path `~/.isolarium/<name>/vm/metadata.json`
    - [ ] Add fallback read logic: try new path first, fall back to old `~/.isolarium/<name>/repo.json`
    - [ ] Update `CleanupHostMetadata()` to clean both old and new paths

---

## Steel Thread 6: Status Command Lists All Environments

- [ ] **Task 6.1: `isolarium status` lists both VMs and containers**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/status/...`
  - Observable: `isolarium status` with no flags lists all environments (VMs and containers) showing name, type, state, and type-specific details (repository/branch for VMs, work directory for containers). `--name` and `--type` flags filter the output.
  - Evidence: Unit tests verify: (1) status output includes both VM and container environments, (2) container status shows work directory from metadata, (3) `--name` filtering works, (4) `--type` filtering works.
  - Steps:
    - [ ] Refactor `internal/status/status.go` to support multiple environment types. Create an `EnvironmentStatus` struct with Name, Type, State, and type-specific fields (Repository/Branch for VMs, WorkDirectory for containers).
    - [ ] Add `ListAllEnvironments()` that scans `~/.isolarium/` directory for all environments and queries their state via the appropriate backend
    - [ ] Update `newStatusCmd()` to accept `--type` flag and display the expanded status format
    - [ ] For container status, read work directory from docker metadata and query container state via `docker inspect`

---

## Steel Thread 7: Docker Availability Check and Error Handling

- [ ] **Task 7.1: `create --type container` fails with clear message when Docker is unavailable**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: When `docker info` fails (Docker not installed or not running), `create --type container` exits with error: "Docker is not installed or not running. Install Docker Desktop (macOS) or Docker Engine (Linux) to use container mode."
  - Evidence: Unit tests verify: (1) `BuildCheckDockerCommand()` produces `["docker", "info"]`, (2) `CheckDockerAvailable()` returns the correct error message when the command fails.
  - Steps:
    - [ ] Implement `CheckDockerAvailable()` in `internal/docker/docker.go` that runs `docker info` and returns a descriptive error if it fails
    - [ ] Ensure `Create()` calls `CheckDockerAvailable()` before proceeding with image build

- [ ] **Task 7.2: Container commands detect stopped containers and provide guidance**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: When a container exists but is stopped, `run` and `shell` commands error with: "Container '<name>' is stopped. Run 'isolarium create --type container' to recreate it."
  - Evidence: Unit tests verify the error message is produced when `GetState()` returns "stopped".
  - Steps:
    - [ ] Add stopped-state detection logic in `Exec()` and `ExecInteractive()` in the docker package
    - [ ] Return a clear, actionable error message guiding the user to recreate

---

## Steel Thread 8: Integration Tests for Container Mode

- [ ] **Task 8.1: Integration tests for container lifecycle (create, exec, destroy)**
  - TaskType: INFRA
  - Entrypoint: `go test -tags=integration ./internal/docker/...`
  - Observable: Integration tests create a real Docker container, execute a command inside it, verify the work directory is mounted, and destroy the container. Tests FAIL (not skip) if Docker is not available.
  - Evidence: `go test -tags=integration ./internal/docker/...` passes on a machine with Docker installed.
  - Steps:
    - [ ] Create `internal/docker/integration_test.go` with `//go:build integration` tag
    - [ ] Write test: create container with a temp directory as work-directory, verify container is running via `docker inspect`, exec `ls /home/isolarium/repo` and verify mounted files are visible, destroy container and verify it's gone
    - [ ] Write test: verify security flags are applied (non-root user, capabilities dropped)
    - [ ] Update `Makefile` to add `test-integration-docker` target: `go test -tags=integration ./internal/docker/...`
    - [ ] Update `test-scripts/test-end-to-end.sh` to run Docker integration tests when Docker is available (parallel to Lima integration test pattern)
