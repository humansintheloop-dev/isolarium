# Isolarium Platform Implementation Plan

## Idea Type

**Type:** C. Platform/infrastructure capability

**Rationale:** As stated in the specification, Isolarium is developer tooling that provides a secure execution environment for autonomous coding agents. It is infrastructure that enables a secure agent execution workflow.

---

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

- NEVER write production code (`cmd/**/*.go`, `internal/**/*.go`) without first writing a failing test
- Before using Write on any `.go` file in `cmd/` or `internal/`, ask: "Do I have a failing test?" If not, write the test first
- When task direction changes mid-implementation, return to TDD PLANNING state and write a test first

### Verification Requirements

- Hard rule: NEVER git commit, git push, or open a PR unless you have successfully run the project's test command and it exits 0
- Hard rule: If running tests is blocked for any reason (including permissions), ALWAYS STOP immediately. Print the failing command, the exact error output, and the permission/path required
- Before committing, ALWAYS print a Verification section containing the exact test command (NOT an ad-hoc command - it must be a proper test command such as `./test-scripts/*.sh`, `go test ./...`, or `make test`), its exit code, and the last 20 lines of output

---

## Steel Thread Overview

| Steel Thread | Description |
|--------------|-------------|
| 1. Foundation | Go CLI skeleton with `status` command, CI pipeline, and basic test infrastructure |
| 2. Credential Storage | `config set/show/delete` commands for GitHub App credentials in macOS Keychain |
| 3. VM Lifecycle | `create` and `destroy` commands for Lima VM management |
| 4. Repository Cloning | Clone repository inside VM using minted GitHub App installation token, checking out host's current branch |
| 5. Script Execution | `run --script` command to execute user scripts inside VM |
| 6. Claude Session | `--copy-session` and `--fresh-login` flags for Claude Code authentication |
| 7. SSH Access | `ssh` command for interactive VM debugging |

---

## Steel Thread 1: Foundation

**Goal:** Establish Go CLI skeleton with a working `status` command, CI pipeline, and test infrastructure.

- [x] **Task 1.1: Go CLI with `status` command reports "no VM" state**
  - TaskType: INFRA
  - Entrypoint: `go build -o isolarium ./cmd/isolarium && ./isolarium status`
  - Observable: CLI outputs `VM: none` and `GitHub App: not configured` with exit code 0
  - Evidence: CI runs `go test ./...` which includes a test that invokes the CLI and asserts the expected output
  - Steps:
    - [x] Create `go.mod` with module `github.com/cer/isolarium` and Go 1.22+
    - [x] Create `cmd/isolarium/main.go` with Cobra CLI setup and `status` subcommand
    - [x] Create `internal/status/status.go` with status checking logic that returns VM state and config state
    - [x] Create `internal/status/status_test.go` that tests the status logic returns correct defaults
    - [x] Create `cmd/isolarium/main_test.go` that executes the binary and asserts output contains expected strings
    - [x] Create `.github/workflows/ci.yml` that runs `go test ./...`
    - [x] Create `.gitignore` for Go binaries and build artifacts

---

## Steel Thread 2: Credential Storage

**Goal:** Implement `config` subcommands for managing GitHub App credentials in macOS Keychain.

- [ ] **Task 2.1: `config set` stores GitHub App credentials in Keychain**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium config set --app-id 12345 --private-key-file ./test-key.pem`
  - Observable: Credentials stored in macOS Keychain under service "isolarium" with account "github-app"; command exits 0 with confirmation message
  - Evidence: Test creates a temp private key file, runs `config set`, then uses `security find-generic-password` to verify the credential exists
  - Steps:
    - [ ] Add `github.com/keybase/go-keychain` dependency to go.mod
    - [ ] Create `internal/keychain/keychain.go` with `StoreCredentials(appID string, privateKey []byte)` function
    - [ ] Create `internal/keychain/keychain_test.go` that tests storing and retrieving credentials
    - [ ] Add `config` command group to CLI with `set` subcommand accepting `--app-id` and `--private-key-file` flags
    - [ ] Create `cmd/isolarium/config_test.go` integration test that runs the full command

- [ ] **Task 2.2: `config show` displays configured GitHub App ID**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium config show`
  - Observable: Outputs `GitHub App ID: 12345` (shows ID but not private key) with exit code 0; outputs "not configured" if no credentials stored
  - Evidence: Test stores credentials via `config set`, then runs `config show` and asserts output contains the app ID
  - Steps:
    - [ ] Add `GetCredentials() (appID string, privateKey []byte, error)` to keychain package
    - [ ] Add `show` subcommand to `config` command group
    - [ ] Create test in `cmd/isolarium/config_test.go` for the show command

- [ ] **Task 2.3: `config delete` removes credentials from Keychain**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium config delete`
  - Observable: Credentials removed from Keychain; command exits 0 with confirmation message; subsequent `config show` reports "not configured"
  - Evidence: Test stores credentials, runs `config delete`, then verifies `config show` reports not configured
  - Steps:
    - [ ] Add `DeleteCredentials()` to keychain package
    - [ ] Add `delete` subcommand to `config` command group
    - [ ] Create test that verifies the full delete flow

- [ ] **Task 2.4: `status` reports GitHub App configuration state**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium status`
  - Observable: Status output includes `GitHub App: configured (App ID: 12345)` when credentials exist, or `GitHub App: not configured` when absent
  - Evidence: Test configures credentials, runs status, asserts "configured" in output; deletes credentials, runs status, asserts "not configured"
  - Steps:
    - [ ] Update `internal/status/status.go` to check Keychain for credentials
    - [ ] Update status_test.go with tests for both configured and unconfigured states

---

## Steel Thread 3: VM Lifecycle

**Goal:** Implement `create` and `destroy` commands for Lima VM management.

- [ ] **Task 3.1: `create` provisions a Lima VM with required toolchain**
  - TaskType: OUTCOME
  - Entrypoint: `cd /path/to/git/repo && ./isolarium create`
  - Observable: Lima VM named "isolarium" created and running; VM contains git, Node.js, Docker, gh CLI, and Claude Code installed; command exits 0
  - Evidence: Test runs `create` in a git repo directory, then runs `limactl list` and asserts VM exists and is running; runs `limactl shell isolarium -- which git node docker gh claude` and asserts all tools found
  - Steps:
    - [ ] Create `internal/lima/lima.go` with `CreateVM()` function
    - [ ] Create `internal/lima/template.yaml` Lima VM configuration with Ubuntu base, Docker, Node.js, git, gh CLI
    - [ ] Add provisioning script to install Claude Code via npm
    - [ ] Create `internal/lima/lima_test.go` unit tests for configuration generation
    - [ ] Add `create` subcommand to CLI that reads current directory git remote and current branch
    - [ ] Create `internal/git/git.go` with `GetRemoteURL()` and `GetCurrentBranch()` functions
    - [ ] Create integration test that provisions VM in a test git repo

- [ ] **Task 3.2: `create` fails gracefully when not in a git repository**
  - TaskType: OUTCOME
  - Entrypoint: `cd /tmp && ./isolarium create`
  - Observable: Command exits with non-zero code and error message "not a git repository"
  - Evidence: Test runs `create` in a non-git directory and asserts exit code is non-zero and stderr contains error message
  - Steps:
    - [ ] Update `create` command to check for git repository before proceeding
    - [ ] Add test for the error case

- [ ] **Task 3.3: `create` fails gracefully when VM already exists**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium create` (when VM already exists)
  - Observable: Command exits with non-zero code and error message "VM already exists"
  - Evidence: Test runs `create` twice; second invocation fails with expected error
  - Steps:
    - [ ] Add VM existence check to `create` command
    - [ ] Add test that creates VM, then attempts second create

- [ ] **Task 3.4: `destroy` deletes the Lima VM completely**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium destroy`
  - Observable: Lima VM "isolarium" stopped and deleted; command exits 0; `limactl list` shows no "isolarium" VM
  - Evidence: Test creates VM, runs `destroy`, then runs `limactl list` and asserts VM is gone
  - Steps:
    - [ ] Create `internal/lima/destroy.go` with `DestroyVM()` function
    - [ ] Add `destroy` subcommand to CLI
    - [ ] Create test that creates and destroys VM

- [ ] **Task 3.5: `destroy` succeeds idempotently when no VM exists**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium destroy` (when no VM exists)
  - Observable: Command exits 0 with message "no VM to destroy"
  - Evidence: Test runs `destroy` when no VM exists and asserts exit code 0
  - Steps:
    - [ ] Update `destroy` to handle missing VM gracefully
    - [ ] Add test for idempotent destroy

- [ ] **Task 3.6: `status` reports VM state (none/running/stopped)**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium status`
  - Observable: Status output includes `VM: running` when VM exists and running, `VM: stopped` when stopped, `VM: none` when absent
  - Evidence: Test checks status with no VM (none), creates VM and checks status (running), stops VM and checks status (stopped)
  - Steps:
    - [ ] Update `internal/status/status.go` to query Lima VM state
    - [ ] Add tests for all three VM states

---

## Steel Thread 4: Repository Cloning

**Goal:** Clone repository inside VM using minted GitHub App installation token.

- [ ] **Task 4.1: `create` mints GitHub App installation token**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium create` (with configured GitHub App and valid installation on repo)
  - Observable: Installation token minted from GitHub API; token used for git clone inside VM
  - Evidence: Test with mock GitHub API verifies token minting flow; integration test with real GitHub App (if available) verifies token is valid
  - Steps:
    - [ ] Add `github.com/google/go-github/v58/github` and `github.com/golang-jwt/jwt/v5` dependencies
    - [ ] Create `internal/github/token.go` with `MintInstallationToken(appID, privateKey, repoOwner, repoName)` function
    - [ ] Create `internal/github/token_test.go` with unit tests using mock HTTP responses
    - [ ] Update `create` command to mint token after VM creation

- [ ] **Task 4.2: `create` clones repository inside VM using token, checking out host's current branch**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium create`
  - Observable: Repository cloned at `/home/lima.linux/repo` inside VM using the minted token; same branch as host checked out; git remote configured with token for push
  - Evidence: Test runs `create` on a feature branch, then runs `limactl shell isolarium -- git -C /home/lima.linux/repo branch --show-current` and asserts it matches the host branch
  - Steps:
    - [ ] Create `internal/lima/clone.go` with `CloneRepo(vm, repoURL, branch, token)` function
    - [ ] Update `create` command to clone repo after token minting, passing the detected branch
    - [ ] Configure git credential helper inside VM for token-based auth
    - [ ] Add integration test that verifies clone completes on correct branch

- [ ] **Task 4.3: `status` reports associated repository**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium status`
  - Observable: Status output includes `Repository: owner/repo` when VM exists with cloned repo
  - Evidence: Test creates VM with repo, runs status, asserts repository name in output
  - Steps:
    - [ ] Store repository metadata in VM (e.g., `/home/lima.linux/.isolarium/repo.json`)
    - [ ] Update status command to read and display repository info
    - [ ] Add test for repository display

---

## Steel Thread 5: Script Execution

**Goal:** Implement `run --script` command to execute user scripts inside VM.

- [ ] **Task 5.1: `run --script` copies and executes script inside VM**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run --script ./agent.sh`
  - Observable: Script copied to VM, executed with attached I/O; stdout/stderr streamed to terminal; exit code propagated
  - Evidence: Test creates VM, creates test script that echoes "hello", runs `run --script`, asserts "hello" appears in output
  - Steps:
    - [ ] Create `internal/lima/exec.go` with `CopyFile(vm, src, dest)` and `ExecScript(vm, scriptPath)` functions
    - [ ] Add `run` subcommand with `--script` flag
    - [ ] Implement I/O streaming using `limactl shell` with attached TTY
    - [ ] Create test with simple echo script

- [ ] **Task 5.2: `run` mints fresh token and injects as environment variable**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run --script ./agent.sh`
  - Observable: Fresh GitHub installation token minted; `GIT_TOKEN` environment variable set inside VM during script execution
  - Evidence: Test creates script that echoes `$GIT_TOKEN`, runs `run --script`, asserts token value appears in output (non-empty, valid format)
  - Steps:
    - [ ] Update `run` command to mint fresh token before execution
    - [ ] Pass token via environment variable to `limactl shell`
    - [ ] Add test that verifies token is available in script environment

- [ ] **Task 5.3: `run` fails gracefully when VM does not exist**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run --script ./agent.sh` (when no VM exists)
  - Observable: Command exits with non-zero code and error message "no VM exists; run 'isolarium create' first"
  - Evidence: Test runs `run` without creating VM, asserts error message and non-zero exit
  - Steps:
    - [ ] Add VM existence check to `run` command
    - [ ] Add test for missing VM error

- [ ] **Task 5.4: `run` handles Ctrl+C to terminate script**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run --script ./long-running.sh` then Ctrl+C
  - Observable: Script receives SIGINT; script terminates; `isolarium run` exits
  - Evidence: Test creates script with sleep loop, runs `run --script` in background, sends SIGINT, asserts process terminates within timeout
  - Steps:
    - [ ] Set up signal handling in `run` command to forward SIGINT to VM process
    - [ ] Create test with long-running script and signal handling

---

## Steel Thread 6: Claude Session

**Goal:** Implement `--copy-session` and `--fresh-login` flags for Claude Code authentication.

- [ ] **Task 6.1: `run --copy-session` copies Claude session from host to VM**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run --script ./agent.sh --copy-session`
  - Observable: Contents of `~/.claude/` from host copied to `/home/lima.linux/.claude/` inside VM; Claude Code in script can authenticate without login prompt
  - Evidence: Test creates mock `~/.claude/` directory with test files, runs `run --copy-session`, verifies files exist in VM via `limactl shell`
  - Steps:
    - [ ] Create `internal/lima/session.go` with `CopyClaudeSession(vm)` function
    - [ ] Add `--copy-session` flag to `run` command (default: true)
    - [ ] Copy `~/.claude/` directory contents to VM before script execution
    - [ ] Add test that verifies session files are copied

- [ ] **Task 6.2: `run --fresh-login` skips session copy for device code flow**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run --script ./agent.sh --fresh-login`
  - Observable: No `~/.claude/` copied from host; Claude Code in VM prompts for device code authentication
  - Evidence: Test runs with `--fresh-login`, verifies `~/.claude/` in VM is empty or absent
  - Steps:
    - [ ] Add `--fresh-login` flag that sets `--copy-session=false`
    - [ ] Ensure `--fresh-login` and `--copy-session` are mutually exclusive
    - [ ] Add test that verifies session is not copied with `--fresh-login`

---

## Steel Thread 7: SSH Access

**Goal:** Implement `ssh` command for interactive VM debugging.

- [ ] **Task 7.1: `ssh` opens interactive shell in VM**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium ssh`
  - Observable: Interactive shell opens inside the Lima VM; user can run commands; exit returns to host shell
  - Evidence: Test runs `ssh` command with stdin containing `echo test && exit`, asserts "test" appears in output
  - Steps:
    - [ ] Create `internal/lima/ssh.go` with `OpenShell(vm)` function using `limactl shell`
    - [ ] Add `ssh` subcommand to CLI
    - [ ] Create test that pipes commands to ssh and verifies output

- [ ] **Task 7.2: `ssh` fails gracefully when VM does not exist**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium ssh` (when no VM exists)
  - Observable: Command exits with non-zero code and error message "no VM exists; run 'isolarium create' first"
  - Evidence: Test runs `ssh` without VM, asserts error message
  - Steps:
    - [ ] Add VM existence check to `ssh` command
    - [ ] Add test for missing VM error

---

## Security Verification Tasks

**Goal:** Verify security properties defined in the specification.

- [ ] **Task 8.1: VM has no host filesystem mounts**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-no-host-mounts.sh`
  - Observable: Lima VM configuration has no `mounts:` entries; `mount` command inside VM shows no host paths
  - Evidence: `test-scripts/test-end-to-end.sh` runs `test-no-host-mounts.sh` which creates VM and verifies no host mounts
  - Steps:
    - [ ] Create `test-scripts/test-no-host-mounts.sh` that inspects Lima config and runs `mount` inside VM
    - [ ] Update Lima template to explicitly disable default mounts
    - [ ] Add script to `test-scripts/test-end-to-end.sh`

- [ ] **Task 8.2: VM has no host Docker socket exposure**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-no-docker-socket.sh`
  - Observable: `/var/run/docker.sock` inside VM is the VM's own Docker daemon socket, not the host's
  - Evidence: `test-scripts/test-end-to-end.sh` runs `test-no-docker-socket.sh` which verifies Docker socket is VM-local
  - Steps:
    - [ ] Create `test-scripts/test-no-docker-socket.sh` that verifies Docker socket ownership
    - [ ] Add script to `test-scripts/test-end-to-end.sh`

- [ ] **Task 8.3: No ambient git credentials in VM**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-no-git-credentials.sh`
  - Observable: `git config --global credential.helper` inside VM is empty or returns non-zero; no `~/.git-credentials` file exists
  - Evidence: `test-scripts/test-end-to-end.sh` runs `test-no-git-credentials.sh` which verifies no global git credentials
  - Steps:
    - [ ] Create `test-scripts/test-no-git-credentials.sh` that checks git credential configuration
    - [ ] Add script to `test-scripts/test-end-to-end.sh`

---

## Test Infrastructure

- [ ] **Task 9.1: Create test-scripts directory structure**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-end-to-end.sh`
  - Observable: test-end-to-end.sh runs test-cleanup.sh and all test scripts in sequence; exits 0 if all pass
  - Evidence: CI runs `./test-scripts/test-end-to-end.sh` and passes
  - Steps:
    - [ ] Create `test-scripts/test-cleanup.sh` that destroys any existing isolarium VM
    - [ ] Create `test-scripts/test-end-to-end.sh` that runs cleanup then individual test scripts
    - [ ] Update `.github/workflows/ci.yml` to run `./test-scripts/test-end-to-end.sh` after `go test ./...`

---

## Change History

### 2026-02-04: Branch handling

Updated to reflect design decision that VM clones/checks out the same branch as the host repo:
- Steel Thread 4 overview: Added "checking out host's current branch"
- Task 3.1: Added `GetCurrentBranch()` function to git detection
- Task 4.2: Updated to clone with branch parameter and verify correct branch checkout
