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
| 2. Basic VM Lifecycle | Basic `create` and `destroy` commands for Lima VM management |
| 3. Credential Storage | GitHub App credentials via environment variables (`GITHUB_APP_ID`, `GITHUB_APP_PRIVATE_KEY_PATH`) |
| 4. Repository Cloning | Clone repository inside VM using minted GitHub App installation token, checking out host's current branch |
| 5. Claude Session | `--copy-session` flag to copy Claude Code session from host to VM |
| 6. Workflow Tools Installation | Clone workflow tools repo and install Claude Code plugins during VM creation |
| 7. Command Execution | `run` command to execute commands inside VM in the repo directory |
| 8. VM Lifecycle Hardening | Error handling and status reporting for VM lifecycle |
| 9. Fresh Login | `--fresh-login` flag for device code authentication flow |
| 10. SSH Access | `ssh` command for interactive VM debugging |
| 11. Security Verification | Verify VM isolation properties (no host mounts, no Docker socket, no git credentials) |
| 12. Test Infrastructure | End-to-end test scripts and CI integration |

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

## Steel Thread 2: Basic VM Lifecycle

**Goal:** Implement basic `create` and `destroy` commands for Lima VM management.

- [x] **Task 2.1: `create` provisions a Lima VM with required toolchain**
  - TaskType: OUTCOME
  - Entrypoint: `cd /path/to/git/repo && ./isolarium create`
  - Observable: Lima VM named "isolarium" created and running; VM contains git, Node.js, Docker, gh CLI, and Claude Code installed; command exits 0
  - Evidence: Test runs `create` in a git repo directory, then runs `limactl list` and asserts VM exists and is running; runs `limactl shell isolarium -- which git node docker gh claude` and asserts all tools found
  - Steps:
    - [x] Create `internal/lima/lima.go` with `CreateVM()` function
    - [x] Create `internal/lima/template.yaml` Lima VM configuration with Ubuntu base, Docker, Node.js, git, gh CLI
    - [x] Add provisioning script to install Claude Code via npm
    - [x] Create `internal/lima/lima_test.go` unit tests for configuration generation
    - [x] Add `create` subcommand to CLI that reads current directory git remote and current branch
    - [x] Create `internal/git/git.go` with `GetRemoteURL()` and `GetCurrentBranch()` functions
    - [x] Create integration test that provisions VM in a test git repo

- [x] **Task 2.2: `destroy` deletes the Lima VM completely**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium destroy`
  - Observable: Lima VM "isolarium" stopped and deleted; command exits 0; `limactl list` shows no "isolarium" VM
  - Evidence: Test creates VM, runs `destroy`, then runs `limactl list` and asserts VM is gone
  - Steps:
    - [x] Create `DestroyVM()` function in `internal/lima/lima.go`
    - [x] Add `destroy` subcommand to CLI
    - [x] Create test that destroys VM and verifies it's gone

---

## Steel Thread 3: Credential Storage

**Goal:** Support GitHub App credentials via environment variables.

**Note:** Originally planned to use macOS Keychain, but pivoted to environment variables due to Keychain ownership/code-signing complexities. Users set `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY_PATH` environment variables directly.

- [x] **Task 3.1: `status` reports GitHub App configuration state from environment variables**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium status`
  - Observable: Status output includes `GitHub App: configured` when both `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY_PATH` environment variables are set, or `GitHub App: not configured` when either is absent
  - Evidence: Tests verify status reports correctly based on environment variable presence
  - Steps:
    - [x] Update `internal/status/status.go` to check `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY_PATH` environment variables
    - [x] Update status_test.go with tests for both configured and unconfigured states (both env vars set, only one set, neither set)

---

## Steel Thread 4: Repository Cloning

**Goal:** Clone repository inside VM using minted GitHub App installation token.

- [x] **Task 4.1: `create` mints GitHub App installation token**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium create` (with configured GitHub App and valid installation on repo)
  - Observable: Installation token minted from GitHub API; token used for git clone inside VM
  - Evidence: Test with mock GitHub API verifies token minting flow; integration test with real GitHub App (if available) verifies token is valid
  - Steps:
    - [x] Add `github.com/golang-jwt/jwt/v5` dependency (used net/http instead of go-github for simplicity)
    - [x] Create `internal/github/url.go` with `ParseRepoURL(remoteURL)` function for SSH/HTTPS parsing
    - [x] Create `internal/github/token.go` with `TokenMinter` and `MintInstallationToken(owner, repo)` function
    - [x] Create `internal/github/token_test.go` with unit tests using mock HTTP responses
    - [x] Update `create` command to mint token after VM creation (when GitHub App configured)

- [x] **Task 4.2: `create` clones repository inside VM using token, checking out host's current branch**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium create`
  - Observable: Repository cloned at `~/repo` inside VM using the minted token; same branch as host checked out; git remote configured with token for push
  - Evidence: Test runs `create` on a feature branch, then runs `limactl shell isolarium -- git -C ~/repo branch --show-current` and asserts it matches the host branch
  - Steps:
    - [x] Create `internal/lima/clone.go` with `CloneRepo(remoteURL, branch, token)` function
    - [x] Update `create` command to clone repo after token minting, passing the detected branch
    - [x] Token embedded in clone URL using `https://x-access-token:TOKEN@github.com/...` format
    - [x] Public repos can be cloned without auth when GitHub App not configured

- [x] **Task 4.3: `status` reports associated repository**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium status`
  - Observable: Status output includes `Repository: owner/repo` when VM exists with cloned repo
  - Evidence: Test creates VM with repo, runs status, asserts repository name in output
  - Steps:
    - [x] Create `internal/lima/metadata.go` to store/read repository metadata in VM at `~/.isolarium/repo.json`
    - [x] Update status command to read and display repository info
    - [x] Add test for repository fields in Status struct

---

## Steel Thread 5: Claude Session

**Goal:** Implement `--copy-session` flag to copy Claude Code credentials from host to VM.

**Environment variable:** `CLAUDE_CREDENTIALS_PATH` - path to credentials file on host

- [x] **Task 5.1: `run --copy-session` copies Claude credentials from host to VM**
  - TaskType: OUTCOME
  - Entrypoint: `CLAUDE_CREDENTIALS_PATH=~/.claude/.credentials.json ./isolarium run --script ./agent.sh --copy-session`
  - Observable: Credentials file copied to VM at `~/.claude/.credentials.json` with permissions `600`; Claude Code in script can authenticate without login prompt
  - Evidence: Test runs `run --copy-session` with a script that executes `claude -p "hello"` and verifies it completes successfully (exit code 0, output contains a greeting response) without prompting for login
  - Steps:
    - [x] Create `internal/lima/session.go` with `CopyClaudeCredentials(credentialsPath)` function
    - [x] Add `--copy-session` flag to `run` command (default: true)
    - [x] Read `CLAUDE_CREDENTIALS_PATH` environment variable
    - [x] Copy credentials file to VM at `~/.claude/.credentials.json` with mode 600
    - [x] Add test that runs `claude -p "hello"` inside VM and verifies successful execution

---

## Steel Thread 6: Workflow Tools Installation

**Goal:** Enhance VM setup to include workflow tools and Claude Code plugins from the humansintheloop-dev repository.

- [x] **Task 6.1: `create` clones workflow tools repository into VM**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium create`
  - Observable: Repository `humansintheloop-dev/humansintheloop-dev-workflow-and-tools` cloned at `~/workflow-tools` inside VM
  - Evidence: Test runs `create`, then runs `limactl shell isolarium -- ls ~/workflow-tools` and asserts directory contains expected files (install-marketplace.sh, reinstall-plugin.sh)
  - Steps:
    - [x] Update `internal/lima/lima.go` `CreateVM()` to clone workflow tools repo after main repo clone
    - [x] Clone using `git clone git@github.com:humansintheloop-dev/humansintheloop-dev-workflow-and-tools.git ~/workflow-tools`
    - [x] Add test that verifies workflow tools directory exists with expected scripts

- [x] **Task 6.2: `create` runs install-marketplace.sh to install marketplace plugins**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium create`
  - Observable: Marketplace plugins installed; install-marketplace.sh exits 0
  - Evidence: Test runs `create`, then verifies marketplace plugins are present in Claude Code configuration
  - Steps:
    - [x] Execute `~/workflow-tools/install-marketplace.sh` after cloning workflow tools
    - [x] Capture and log output from install script
    - [x] Add test that verifies installation completed

- [x] **Task 6.3: `create` runs reinstall-plugin.sh to install custom plugins**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium create`
  - Observable: Custom plugins installed into Claude Code; reinstall-plugin.sh exits 0
  - Evidence: Test runs `create`, then verifies custom plugins are present in Claude Code plugin directory
  - Steps:
    - [x] Execute `~/workflow-tools/reinstall-plugin.sh` after marketplace installation
    - [x] Capture and log output from reinstall script
    - [x] Add test that verifies plugins are installed

---

## Steel Thread 7: Command Execution

**Goal:** Implement `run` command to execute commands inside the VM in the repo directory.

**Syntax:** `isolarium run [options] -- command arg...`

The command runs directly inside the VM in `~/repo` — no files are copied from the host. By default the command runs non-interactively (stdout/stderr streamed). The `--interactive`/`-i` flag enables TTY mode for commands that need user interaction (e.g., Claude Code).

- [x] **Task 7.1: `run` executes a command inside the VM in the repo directory**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run -- echo hello`
  - Observable: Command executed inside VM in `~/repo` directory; stdout/stderr streamed to terminal; exit code propagated
  - Evidence: Test creates VM, runs `run -- echo hello`, asserts "hello" appears in output and exit code is 0; runs `run -- pwd`, asserts output is `/home/<user>.linux/repo`
  - Steps:
    - [x] Create `internal/lima/exec.go` with `ExecCommand(vm, workdir, args)` function using `limactl shell`
    - [x] Add `run` subcommand that takes args after `--`
    - [x] Set working directory to `~/repo` for command execution
    - [x] Stream stdout/stderr to terminal (non-interactive by default)
    - [x] Propagate command exit code
    - [x] Create test with simple echo command

- [x] **Task 7.2: `run --interactive` enables TTY mode for user interaction**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run -i -- claude`
  - Observable: Command runs with TTY attached via `limactl shell --tty`; user can interact with the command
  - Evidence: Test runs `run -i -- cat` with stdin piped, verifies input is echoed back (TTY mode active)
  - Steps:
    - [x] Add `--interactive`/`-i` flag to `run` command
    - [x] When `-i` is set, use `limactl shell --tty` to attach TTY
    - [x] Connect stdin/stdout/stderr for interactive use
    - [x] Create test that verifies interactive mode works

- [x] **Task 7.3: `run` mints fresh token and injects as environment variable**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run -- printenv GIT_TOKEN`
  - Observable: Fresh GitHub installation token minted; `GIT_TOKEN` environment variable set inside VM during command execution
  - Evidence: Test runs `run -- printenv GIT_TOKEN`, asserts token value appears in output (non-empty, valid format)
  - Steps:
    - [x] Update `run` command to mint fresh token before execution
    - [x] Pass token via environment variable to `limactl shell`
    - [x] Add test that verifies token is available in command environment

- [x] **Task 7.4: `run` fails gracefully when VM does not exist**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run -- echo hello` (when no VM exists)
  - Observable: Command exits with non-zero code and error message "no VM exists; run 'isolarium create' first"
  - Evidence: Test runs `run` without creating VM, asserts error message and non-zero exit
  - Steps:
    - [x] Add VM existence check to `run` command
    - [x] Add test for missing VM error

- [x] **Task 7.5: `run` handles Ctrl+C to terminate command**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run -- sleep 3600` then Ctrl+C
  - Observable: Command receives SIGINT; command terminates; `isolarium run` exits
  - Evidence: Test runs `run -- sleep 3600` in background, sends SIGINT, asserts process terminates within timeout
  - Steps:
    - [x] Set up signal handling in `run` command to forward SIGINT to VM process
    - [x] Create test with long-running command and signal handling

---

## Steel Thread 8: VM Lifecycle Hardening

**Goal:** Add error handling and status reporting for VM lifecycle.

- [x] **Task 8.1: `create` fails gracefully when not in a git repository**
  - TaskType: OUTCOME
  - Entrypoint: `cd /tmp && ./isolarium create`
  - Observable: Command exits with non-zero code and error message "not a git repository"
  - Evidence: Test runs `create` in a non-git directory and asserts exit code is non-zero and stderr contains error message
  - Steps:
    - [x] Update `create` command to check for git repository before proceeding
    - [x] Add test for the error case

- [x] **Task 8.2: `create` fails gracefully when VM already exists**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium create` (when VM already exists)
  - Observable: Command exits with non-zero code and error message "VM already exists"
  - Evidence: Test runs `create` twice; second invocation fails with expected error
  - Steps:
    - [x] Add VM existence check to `create` command
    - [x] Add test that creates VM, then attempts second create

- [ ] **Task 8.3: `destroy` succeeds idempotently when no VM exists**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium destroy` (when no VM exists)
  - Observable: Command exits 0 with message "no VM to destroy"
  - Evidence: Test runs `destroy` when no VM exists and asserts exit code 0
  - Steps:
    - [ ] Update `destroy` to handle missing VM gracefully
    - [ ] Add test for idempotent destroy

- [ ] **Task 8.4: `status` reports VM state (none/running/stopped)**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium status`
  - Observable: Status output includes `VM: running` when VM exists and running, `VM: stopped` when stopped, `VM: none` when absent
  - Evidence: Test checks status with no VM (none), creates VM and checks status (running), stops VM and checks status (stopped)
  - Steps:
    - [ ] Update `internal/status/status.go` to query Lima VM state
    - [ ] Add tests for all three VM states

---

## Steel Thread 9: Fresh Login

**Goal:** Implement `--fresh-login` flag for device code authentication flow.

- [ ] **Task 9.1: `run --fresh-login` skips session copy for device code flow**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium run --script ./agent.sh --fresh-login`
  - Observable: No `~/.claude/` copied from host; Claude Code in VM prompts for device code authentication
  - Evidence: Test runs with `--fresh-login`, verifies `~/.claude/` in VM is empty or absent
  - Steps:
    - [ ] Add `--fresh-login` flag that sets `--copy-session=false`
    - [ ] Ensure `--fresh-login` and `--copy-session` are mutually exclusive
    - [ ] Add test that verifies session is not copied with `--fresh-login`

---

## Steel Thread 10: SSH Access

**Goal:** Implement `ssh` command for interactive VM debugging.

- [ ] **Task 10.1: `ssh` opens interactive shell in VM**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium ssh`
  - Observable: Interactive shell opens inside the Lima VM; user can run commands; exit returns to host shell
  - Evidence: Test runs `ssh` command with stdin containing `echo test && exit`, asserts "test" appears in output
  - Steps:
    - [ ] Create `internal/lima/ssh.go` with `OpenShell(vm)` function using `limactl shell`
    - [ ] Add `ssh` subcommand to CLI
    - [ ] Create test that pipes commands to ssh and verifies output

- [ ] **Task 10.2: `ssh` fails gracefully when VM does not exist**
  - TaskType: OUTCOME
  - Entrypoint: `./isolarium ssh` (when no VM exists)
  - Observable: Command exits with non-zero code and error message "no VM exists; run 'isolarium create' first"
  - Evidence: Test runs `ssh` without VM, asserts error message
  - Steps:
    - [ ] Add VM existence check to `ssh` command
    - [ ] Add test for missing VM error

---

## Steel Thread 11: Security Verification

**Goal:** Verify security properties defined in the specification.

- [ ] **Task 11.1: VM has no host filesystem mounts**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-no-host-mounts.sh`
  - Observable: Lima VM configuration has no `mounts:` entries; `mount` command inside VM shows no host paths
  - Evidence: `test-scripts/test-end-to-end.sh` runs `test-no-host-mounts.sh` which creates VM and verifies no host mounts
  - Steps:
    - [ ] Create `test-scripts/test-no-host-mounts.sh` that inspects Lima config and runs `mount` inside VM
    - [ ] Update Lima template to explicitly disable default mounts
    - [ ] Add script to `test-scripts/test-end-to-end.sh`

- [ ] **Task 11.2: VM has no host Docker socket exposure**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-no-docker-socket.sh`
  - Observable: `/var/run/docker.sock` inside VM is the VM's own Docker daemon socket, not the host's
  - Evidence: `test-scripts/test-end-to-end.sh` runs `test-no-docker-socket.sh` which verifies Docker socket is VM-local
  - Steps:
    - [ ] Create `test-scripts/test-no-docker-socket.sh` that verifies Docker socket ownership
    - [ ] Add script to `test-scripts/test-end-to-end.sh`

- [ ] **Task 11.3: No ambient git credentials in VM**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-no-git-credentials.sh`
  - Observable: `git config --global credential.helper` inside VM is empty or returns non-zero; no `~/.git-credentials` file exists
  - Evidence: `test-scripts/test-end-to-end.sh` runs `test-no-git-credentials.sh` which verifies no global git credentials
  - Steps:
    - [ ] Create `test-scripts/test-no-git-credentials.sh` that checks git credential configuration
    - [ ] Add script to `test-scripts/test-end-to-end.sh`

---

## Steel Thread 12: Test Infrastructure

- [ ] **Task 12.1: Create test-scripts directory structure**
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

### 2026-02-04: Reordered steel threads to prioritize basic VM lifecycle

Moved basic VM create/destroy tasks before credential storage to enable earlier VM testing:
- New Steel Thread 2: Basic VM Lifecycle (tasks 2.1-2.2, formerly 3.1 and 3.4)
- Steel Thread 3: Credential Storage (formerly Steel Thread 2, tasks renumbered 3.1-3.4)
- New Steel Thread 4: VM Lifecycle Hardening (remaining VM tasks, formerly 3.2, 3.3, 3.5, 3.6)
- All subsequent steel threads renumbered (5-8 for main features, 9 for security, 10 for test infra)

### 2026-02-04: Branch handling

Updated to reflect design decision that VM clones/checks out the same branch as the host repo:
- Steel Thread 5 overview: Added "checking out host's current branch"
- Task 2.1: Added `GetCurrentBranch()` function to git detection
- Task 5.2: Updated to clone with branch parameter and verify correct branch checkout

### 2026-02-04: Pivoted from Keychain to environment variables for credentials

Due to macOS Keychain ownership and code-signing complexities (errSecInvalidOwnerEdit when different binaries try to access the same credentials), pivoted Steel Thread 3 from Keychain-based storage to environment variables:
- Removed `config set/show/delete` commands
- Users now set `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY_PATH` environment variables directly
- `status` command checks these environment variables to report configuration state
- Simplified from 4 tasks to 1 task (Task 3.1)

### 2026-02-05: Improved Task 7.1 verification to use functional test

Updated Task 7.1 evidence to verify session copying actually works by running Claude Code:
- Changed from checking file existence to running `claude -p "hello"` inside VM
- Test verifies Claude Code executes successfully (exit code 0, output contains a greeting response) without login prompt
- This provides stronger evidence that the session copy is functional, not just present

### 2026-02-05: Reordered and renumbered steel threads

Reordered threads to prioritize core functionality before hardening, and extracted Fresh Login into its own thread:
- Thread 4: Repository Cloning (formerly Thread 5)
- Thread 5: Claude Session (formerly Thread 7, Fresh Login extracted)
- Thread 6: Script Execution (unchanged)
- Thread 7: VM Lifecycle Hardening (formerly Thread 4)
- Thread 8: Fresh Login (extracted from Thread 7)
- Thread 9: SSH Access (formerly Thread 8)
- Thread 10: Security Verification (formerly unnumbered)
- Thread 11: Test Infrastructure (formerly unnumbered)
- All task numbers updated to match their parent thread

### 2026-02-05: Revised Claude credentials handling

Changed from copying entire `~/.claude/` directory to copying only the credentials file:
- New environment variable `CLAUDE_CREDENTIALS_PATH` specifies credentials file on host
- File is copied to VM at `~/.claude/.credentials.json` with permissions 600 (owner read/write only)
- This is more secure and avoids copying unnecessary data (conversation history, settings, etc.)

### 2026-02-05: Added Workflow Tools Installation thread

Added new Steel Thread 6 to install workflow tools and Claude Code plugins during VM creation:
- Clone `git@github.com:humansintheloop-dev/humansintheloop-dev-workflow-and-tools.git` to `~/workflow-tools`
- Run `install-marketplace.sh` to install marketplace plugins
- Run `reinstall-plugin.sh` to install custom plugins into Claude Code
- Renumbered subsequent threads: Script Execution (6→7), VM Lifecycle Hardening (7→8), Fresh Login (8→9), SSH Access (9→10), Security Verification (10→11), Test Infrastructure (11→12)
- Updated Steel Thread Overview table to include all threads (was missing Security Verification and Test Infrastructure)

### 2026-02-06: Revised Steel Thread 7 from script execution to command execution

Redesigned the `run` command interface:
- Changed syntax from `isolarium run --script <path>` to `isolarium run [options] -- command arg...`
- Commands run directly inside the VM in `~/repo` — no file copying from host
- Non-interactive by default (stdout/stderr streamed)
- Added `--interactive`/`-i` flag for TTY mode via `limactl shell --tty` (for commands needing user interaction like Claude Code)
- Renamed thread from "Script Execution" to "Command Execution"
- Added new Task 7.2 for interactive mode; renumbered remaining tasks (old 7.2→7.3, 7.3→7.4, new 7.5 for Ctrl+C)

### 2026-02-08 13:46 - mark-task-complete
Implemented run command with -- command syntax, ExecCommand function, unit tests, and CLI tests

### 2026-02-08 13:48 - mark-task-complete
Added --interactive/-i flag with TTY mode using limactl shell --tty, ExecInteractiveCommand function, and tests

### 2026-02-08 13:56 - mark-task-complete
Added env var injection via command-line env prefix, run command mints fresh token when GitHub App configured

### 2026-02-08 13:58 - mark-task-complete
VM existence check was already implemented in Task 7.1; added conditional test that skips when VM exists

### 2026-02-08 13:59 - mark-task-complete
SIGINT propagation works natively via os/exec on Unix; added integration test verifying process terminates on SIGINT

### 2026-02-08 14:00 - mark-task-complete
Already implemented and tested in earlier steel threads

### 2026-02-08 14:00 - mark-task-complete
Already implemented in CreateVM; added CLI test that verifies error when VM exists
