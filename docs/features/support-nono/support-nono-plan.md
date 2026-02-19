Now I have a thorough understanding of the codebase. Let me generate the plan.

---

# Support Nono Sandbox — Implementation Plan

## Idea Type

**Type A: User-facing feature** — Adds nono as a third isolation backend to isolarium, providing process-level capability-based sandboxing on macOS.

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

- NEVER write production code (`internal/**/*.go` excluding `*_test.go`) without first writing a failing test
- Before using Write on any `.go` file in `internal/` (non-test), ask: "Do I have a failing test?" If not, write the test first
- When task direction changes mid-implementation, return to TDD PLANNING state and write a test first

### Verification Requirements

- Hard rule: NEVER git commit, git push, or open a PR unless you have successfully run the project's test command and it exits 0
- Hard rule: If running tests is blocked for any reason (including permissions), ALWAYS STOP immediately. Print the failing command, the exact error output, and the permission/path required
- Before committing, ALWAYS print a Verification section containing the exact test command (NOT an ad-hoc command - it must be `go test ./...`), its exit code, and the last 20 lines of output

## Overview

This plan adds nono as a third isolation backend to isolarium. Nono provides process-level capability-based sandboxing on macOS, wrapping commands with fine-grained filesystem and command restrictions. Unlike the VM and container backends which create separate environments, nono wraps commands on the host with permission constraints.

The implementation follows the established `DockerBackend` pattern: a `NonoBackend` struct with injected function fields for testability, delegating to an `internal/nono/` package for nono-specific operations.

### Key architectural decisions

- `NonoBackend` follows the `DockerBackend` dependency-injection pattern (injected function fields), not the `LimaBackend` zero-field pattern, for testability
- `internal/nono/` package mirrors `internal/docker/` structure: metadata.go, create.go, destroy.go, command.go, exec.go, shell.go
- Reuses `ExecFunc` and `ShellFunc` types from `internal/backend/docker_backend.go`
- Nono exec/shell functions build command lines and exec on the host with stdio streaming (same pattern as docker/exec.go)
- CI: Existing `go test ./...` in `test-scripts/test-end-to-end.sh` automatically picks up all new Go unit tests

### Files to create

| File | Purpose |
|------|---------|
| `internal/nono/metadata.go` | `NonoMetadata` struct and `MetadataStore` |
| `internal/nono/create.go` | `Creator` struct (validates nono, writes metadata) |
| `internal/nono/destroy.go` | `Destroyer` struct (removes metadata dir) |
| `internal/nono/permissions.go` | Hardcoded permission flag builder |
| `internal/nono/command.go` | `BuildRunCommand`, `BuildRunCommandInteractive`, `BuildShellCommand` |
| `internal/nono/exec.go` | `ExecCommand`, `ExecInteractiveCommand` |
| `internal/nono/shell.go` | `OpenShell` |
| `internal/backend/nono_backend.go` | `NonoBackend` implementing `Backend` interface |
| `internal/nono/metadata_test.go` | Tests for metadata store |
| `internal/nono/create_test.go` | Tests for creator |
| `internal/nono/destroy_test.go` | Tests for destroyer |
| `internal/nono/command_test.go` | Tests for command building |
| `internal/backend/nono_backend_test.go` | Tests for NonoBackend |

### Files to modify

| File | Change |
|------|--------|
| `internal/cli/environment_type.go` | Add `"nono"` to `Set()` validation |
| `internal/backend/resolve.go` | Add `case "nono"` + `newNonoBackend()` factory |
| `internal/backend/resolve_env.go` | Add `"nono"` to `knownEnvironmentTypes` |
| `internal/cli/cmd_create.go` | Add `defaultNonoName`, nono case in `resolveDefaultName()`, `--work-directory` rejection, nono routing |
| `internal/cli/cmd_run.go` | Add nono routing with `runInNono()`, flag rejection |
| `internal/cli/cmd_shell.go` | Add nono routing, `--copy-session` rejection |
| `internal/cli/cmd_destroy.go` | Nono routing (already works via Backend interface for non-VM types) |
| `internal/status/environment.go` | Add `"nono"` to `knownTypes`, nono case in `populateTypeSpecificFields()` |
| `internal/cli/cmd_status.go` | Add `case "nono"` to `formatDetails()` |
| `internal/cli/cmd_clone_repo.go` | Add `--type nono` rejection |
| `internal/cli/cmd_install_tools.go` | Add `--type nono` rejection |
| `internal/cli/cmd_install_workflow_tools_from_source.go` | Add `--type nono` rejection |
| `internal/backend/resolve_test.go` | Add nono resolution test |
| `internal/cli/cmd_create_test.go` | Add nono create tests |
| `internal/cli/cmd_run_test.go` | Add nono run tests |
| `internal/cli/cmd_shell_test.go` | Add nono shell tests |
| `internal/cli/cmd_destroy_test.go` | Add nono destroy tests |
| `internal/cli/cmd_status_test.go` | Add nono status tests |

### Hardcoded permission set (FR7)

All `nono run` and `nono shell` commands include these flags:

```
--allow .
--allow ~/.claude/
--allow-file ~/.claude.json
--allow ~/.claude.json.lock
--allow '~/.claude.json.tmp.*'
--read-file ~/Library/Keychains/login.keychain-db
--allow ~/.cache/uv
--read ~/.local/share/uv
```

Steps should be implemented using TDD.

---

## Steel Thread 1: Create and Non-interactive Run

This thread proves the end-to-end architecture: CLI type validation -> backend resolution -> NonoBackend -> nono package -> metadata storage -> command execution. CI already exists (`.github/workflows/ci.yml` runs `go test ./...` via `test-scripts/test-end-to-end.sh`), so all new Go tests are automatically validated on every commit.

- [ ] **Task 1.1: `isolarium create --type nono` registers type, validates nono, and writes metadata**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./...`
  - Observable: `--type nono` accepted as valid type, resolves to `NonoBackend`; `isolarium create --type nono` validates nono is installed (via `nono --version`), writes metadata to `~/.isolarium/isolarium-nono/nono/metadata.json` with type `"nono"`, work_directory (CWD), and created_at timestamp; `GetState` returns `"configured"` when metadata dir exists and `"none"` otherwise; default name is `isolarium-nono`; `CopyCredentials` is a no-op returning nil; `--work-directory` rejected with `--work-directory is not supported with --type nono`; nono not installed produces `nono is not installed. Install nono to use sandbox mode.`
  - Evidence: Go tests verify type validation, backend resolution, create flow, metadata content, state, default name, CopyCredentials no-op, --work-directory rejection, and nono-not-installed error; `go test ./...` exits 0
  - Steps:
    - [ ] Add `"nono"` to `environmentType.Set()` validation in `internal/cli/environment_type.go` (update error message to include nono)
    - [ ] Create `internal/nono/metadata.go` with `NonoMetadata` struct (`Type`, `WorkDirectory`, `CreatedAt`) and `MetadataStore` (following `internal/docker/metadata.go` pattern with dir path `{name}/nono/`)
    - [ ] Create `internal/nono/create.go` with `Creator` struct (fields: `Runner command.Runner`, `MetadataDir string`) and `Create(name, workDir string) error` that checks nono availability via Runner and writes metadata
    - [ ] Create `internal/backend/nono_backend.go` with `NonoBackend` struct (fields: `Runner`, `MetadataDir`, `ExecFunc`, `ExecInteractiveFunc`, `OpenShellFunc`) implementing all 7 `Backend` methods; `Create` delegates to `nono.Creator`; `GetState` checks metadata dir existence; `CopyCredentials` returns nil; `Exec`/`ExecInteractive`/`OpenShell`/`Destroy` can initially return `UnsupportedOperationError` (to be implemented in later tasks)
    - [ ] Add `case "nono"` to `ResolveBackend()` in `internal/backend/resolve.go` with `newNonoBackend()` factory function
    - [ ] Add `"nono"` to `knownEnvironmentTypes` in `internal/backend/resolve_env.go`
    - [ ] Add `defaultNonoName = "isolarium-nono"` constant and `case "nono"` to `resolveDefaultName()` in `internal/cli/cmd_create.go`
    - [ ] Add nono routing in `cmd_create.go` `RunE`: reject `--work-directory` when explicitly set for nono; route nono through Backend interface (same as container path)
    - [ ] Update `--type` flag description in `internal/cli/root.go` to include `"nono"`

- [ ] **Task 1.2: `isolarium run --type nono -- <cmd>` wraps with nono run and hardcoded permission set**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./...`
  - Observable: `isolarium run --type nono -- echo hello` builds and executes `nono run --allow . --allow ~/.claude/ --allow-file ~/.claude.json --allow ~/.claude.json.lock --allow '~/.claude.json.tmp.*' --read-file ~/Library/Keychains/login.keychain-db --allow ~/.cache/uv --read ~/.local/share/uv -- echo hello`; exit code from the command is propagated; `--copy-session` rejected when explicitly set with `--copy-session is not supported with --type nono`; `--fresh-login` rejected when explicitly set with `--fresh-login is not supported with --type nono`; no credential copy or GitHub token injection for nono
  - Evidence: Go tests verify command building produces correct nono args with all permission flags, CLI routing calls `backend.Exec` with correct name and args, exit code propagation, --copy-session rejection, --fresh-login rejection, and that no CopyCredentials call is made; `go test ./...` exits 0
  - Steps:
    - [ ] Create `internal/nono/permissions.go` with a function that returns the hardcoded permission flag slice (`[]string{"--allow", ".", "--allow", "~/.claude/", ...}`)
    - [ ] Create `internal/nono/command.go` with `BuildRunCommand(args []string) []string` that returns `["nono", "run", <permission-flags>, "--", <args>...]`
    - [ ] Create `internal/nono/exec.go` with `ExecCommand(name string, envVars map[string]string, args []string) (int, error)` that builds the nono run command, creates an `os/exec.Cmd` with inherited env + envVars, streams stdin/stdout/stderr, and returns the exit code
    - [ ] Wire `NonoBackend.Exec` to delegate to `nono.ExecCommand` via the injected `ExecFunc`
    - [ ] Add `runInNono(name string, args []string, interactive bool, resolver BackendResolver) error` function in `internal/cli/cmd_run.go` that resolves the backend, calls `b.Exec` with empty envVars, and propagates exit codes
    - [ ] Add nono routing in `cmd_run.go` `RunE`: before calling `runInNono`, reject `--copy-session` if `cmd.Flags().Changed("copy-session")` and reject `--fresh-login` if `cmd.Flags().Changed("fresh-login")`; otherwise route to `runInNono`

## Steel Thread 2: Interactive Run and Shell

Adds interactive execution modes: `--exec` flag for TTY preservation in `nono run`, and `nono shell` for sandboxed interactive shells.

- [ ] **Task 2.1: `isolarium run -i --type nono -- claude` adds --exec for TTY preservation**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./...`
  - Observable: `isolarium run -i --type nono -- claude` builds `nono run --allow . ... --exec -- claude` (with `--exec` before `--`); non-interactive run does NOT include `--exec`
  - Evidence: Go tests verify `BuildRunCommandInteractive` includes `--exec` in the command, `BuildRunCommand` does not include `--exec`, CLI routing calls `backend.ExecInteractive` when `-i` is set; `go test ./...` exits 0
  - Steps:
    - [ ] Add `BuildRunCommandInteractive(args []string) []string` to `internal/nono/command.go` that includes `--exec` flag before `--`
    - [ ] Add `ExecInteractiveCommand(name string, envVars map[string]string, args []string) (int, error)` to `internal/nono/exec.go` using the interactive command builder
    - [ ] Wire `NonoBackend.ExecInteractive` to delegate to `nono.ExecInteractiveCommand` via the injected `ExecInteractiveFunc`
    - [ ] Update `runInNono` in `cmd_run.go` to call `b.ExecInteractive` when `interactive` is true

- [ ] **Task 2.2: `isolarium shell --type nono` opens sandboxed interactive shell**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./...`
  - Observable: `isolarium shell --type nono` builds and executes `nono shell --allow . --allow ~/.claude/ ...` with the hardcoded permission set (uses `nono shell`, not `nono run`); `--copy-session` rejected when explicitly set with `--copy-session is not supported with --type nono`
  - Evidence: Go tests verify `BuildShellCommand` produces correct `nono shell` args with all permission flags, CLI routing calls `backend.OpenShell`, and --copy-session rejection for nono; `go test ./...` exits 0
  - Steps:
    - [ ] Add `BuildShellCommand() []string` to `internal/nono/command.go` that returns `["nono", "shell", <permission-flags>]`
    - [ ] Create `internal/nono/shell.go` with `OpenShell(name string, envVars map[string]string) (int, error)` that builds the nono shell command and executes with stdio streaming
    - [ ] Wire `NonoBackend.OpenShell` to delegate to `nono.OpenShell` via the injected `OpenShellFunc`
    - [ ] Add nono routing in `cmd_shell.go`: reject `--copy-session` if explicitly changed for nono; skip credential copy and GitHub token injection for nono; call `b.OpenShell`

## Steel Thread 3: Lifecycle Management

Adds environment lifecycle operations: status listing shows nono environments, destroy removes metadata.

- [ ] **Task 3.1: `isolarium status` shows nono environments with "configured" state**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./...`
  - Observable: `isolarium status` lists nono environments alongside VM and container environments; nono environments show state `"configured"` and the work directory in the details column
  - Evidence: Go tests verify nono appears in `ListAllEnvironments` output, state is `"configured"`, work directory is populated from metadata, and `formatDetails` returns the work directory for nono; `go test ./...` exits 0
  - Steps:
    - [ ] Add `"nono"` to `knownTypes` in `internal/status/environment.go`
    - [ ] Add `case "nono"` to `populateTypeSpecificFields()` in `internal/status/environment.go` that reads `work_directory` from `metadata.json` (same pattern as `case "container"`)
    - [ ] Add `case "nono"` to `formatDetails()` in `internal/cli/cmd_status.go` that returns `env.WorkDirectory`

- [ ] **Task 3.2: `isolarium destroy --type nono` removes nono metadata**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./...`
  - Observable: `isolarium destroy --type nono` removes the `~/.isolarium/{name}/nono/` directory; after destroy, the environment no longer appears in status listing
  - Evidence: Go tests verify Destroyer removes the metadata directory and backend.Destroy is called with the correct name; `go test ./...` exits 0
  - Steps:
    - [ ] Create `internal/nono/destroy.go` with `Destroyer` struct (field: `MetadataDir string`) and `Destroy(name string) error` that removes `{MetadataDir}/{name}/nono/` using `os.RemoveAll`
    - [ ] Wire `NonoBackend.Destroy` to delegate to `nono.Destroyer`
    - [ ] Verify nono routing in `cmd_destroy.go` already works via the existing non-VM Backend interface path (same as container)

## Steel Thread 4: VM-only Command Validation

Ensures VM-only commands error with clear messages when used with `--type nono`.

- [ ] **Task 4.1: VM-only commands reject --type nono with clear error messages**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./...`
  - Observable: `isolarium clone-repo --type nono` errors with `clone-repo is not supported with --type nono`; `isolarium install-tools --type nono` errors with `install-tools is not supported with --type nono`; `isolarium install-workflow-tools-from-source --type nono` errors with `install-workflow-tools-from-source is not supported with --type nono`
  - Evidence: Go tests verify each command produces the specified error message when `--type nono` is set; `go test ./...` exits 0
  - Steps:
    - [ ] Add type check at the start of `cmd_clone_repo.go` `RunE`: if `--type` flag is set to `"nono"`, return the error message; requires passing `typeFlag` to the command constructor (update signature to match other commands)
    - [ ] Add type check at the start of `cmd_install_tools.go` `RunE`: same pattern
    - [ ] Add type check at the start of `cmd_install_workflow_tools_from_source.go` `RunE`: same pattern
    - [ ] Update `root.go` to pass `typeFlag` to the VM-only command constructors

## Change History

(No changes yet)
