# Isolarium Platform Capability Specification

## Classification

**Type:** C. Platform/infrastructure capability

**Rationale:** Isolarium is developer tooling that provides a secure execution environment for autonomous coding agents. It is not a user-facing feature (no end-users beyond the developer), not an architecture POC (the architecture is defined), and not an educational example. It is infrastructure that enables a secure agent execution workflow.

---

## 1. Purpose and Context

### 1.1 Problem Statement

Running autonomous coding agents (like Claude Code) on a developer workstation poses security risks:

- Agents may execute arbitrary code, including untrusted dependencies
- Agents with broad filesystem or credential access can cause unintended damage
- A compromised agent could access credentials for multiple repositories
- Audit trails may not clearly distinguish agent actions from human actions

### 1.2 Solution Overview

Isolarium is a CLI tool that provides a secure, isolated execution environment for Claude Code on macOS. It leverages:

- **Lima VMs** for strong isolation from the host
- **GitHub App installation tokens** for repo-scoped, short-lived credentials
- **Disposable VM model** for recovery from compromise
- **Session-based lifecycle** for practical developer workflows

### 1.3 Core Principles

1. **VM compromise is acceptable; host compromise is not**
2. **Credentials are short-lived and scoped to a single repository**
3. **The agent identity is separate from the developer identity**
4. **Recovery is achieved by destroying and recreating the VM**

---

## 2. Consumers

| Consumer | Usage |
|----------|-------|
| Individual developers | Run Claude Code securely on personal machines against private repositories |
| Development teams | Standardize secure agent execution across team members |

---

## 3. Capabilities and Behaviors

### 3.1 VM Lifecycle Management

| Capability | Description |
|------------|-------------|
| Create VM | Provision a new Lima VM with the required toolchain |
| Destroy VM | Delete the VM and all its contents |
| Query status | Report VM state, associated repository, and credential status |
| SSH access | Open an interactive shell in the VM for debugging |

**Lifecycle model:** Session-based. The VM persists across multiple `run` invocations until explicitly destroyed. Only one VM is supported initially.

### 3.2 Credential Management

| Capability | Description |
|------------|-------------|
| Store GitHub App credentials | Save App ID and private key in macOS Keychain (iCloud-synced) |
| Mint installation tokens | Generate short-lived, repo-scoped tokens at runtime |
| Inject credentials into VM | Pass tokens to the VM without persisting to disk |

### 3.3 Claude Code Authentication

| Capability | Description |
|------------|-------------|
| Copy credentials from host | Copy credentials file from `CLAUDE_CREDENTIALS_PATH` to VM `~/.claude/.credentials.json` (default) |
| Fresh login | User authenticates via device code flow inside VM (separate session) |

**Flag:** `--copy-session` (default) or `--fresh-login`

**Environment variable:** `CLAUDE_CREDENTIALS_PATH` - path to credentials file on host

### 3.4 Repository Handling

| Capability | Description |
|------------|-------------|
| Detect repository | Read remote URL and current branch from host's current git directory |
| Clone inside VM | Clone the repository into the VM filesystem using the minted token, checking out the same branch as the host |

### 3.5 Command Execution

| Capability | Description |
|------------|-------------|
| Execute command | Run a command inside the VM in the repo directory |
| Non-interactive mode (default) | Stream stdout/stderr to terminal; Ctrl+C stops the command |
| Interactive mode (`-i`) | Attach TTY for commands that need user interaction (e.g., Claude Code) |

---

## 4. CLI Interface

### 4.1 Commands

```
isolarium create          # Create and start VM for current repo
isolarium run [opts] -- cmd  # Run command in VM repo directory
isolarium destroy         # Delete the VM
isolarium status          # Show VM state, repo, credentials status
isolarium config          # Manage GitHub App credentials in Keychain
isolarium shell             # Open interactive shell in VM
```

### 4.2 Command Details

#### `isolarium create`

Creates a new Lima VM configured for the repository in the current directory.

**Preconditions:**
- Current directory is a git checkout with a GitHub remote
- GitHub App credentials are configured via `isolarium config`
- No VM currently exists

**Actions:**
1. Read repository remote URL and current branch from current directory
2. Retrieve GitHub App credentials from Keychain
3. Create and start Lima VM from predefined configuration
4. Mint GitHub App installation token for the repository
5. Clone repository inside VM using the token, checking out the detected branch
6. Configure git credentials inside VM

**Outputs:**
- VM running and ready
- Repository cloned at known path inside VM, on the same branch as host

#### `isolarium run [options] -- command arg...`

Runs a command inside the VM in the repo directory (`~/repo`).

**Flags:**
| Flag | Description | Default |
|------|-------------|---------|
| `--interactive`, `-i` | Attach TTY for interactive commands | false |
| `--copy-session` | Copy Claude Code session from host | true |
| `--fresh-login` | Use device code flow for fresh Claude session | false |

**Preconditions:**
- VM exists and is running

**Actions:**
1. If `--copy-session`: copy credentials file from `CLAUDE_CREDENTIALS_PATH` to VM `~/.claude/.credentials.json` (mode 600)
2. Mint fresh GitHub App installation token (tokens are short-lived)
3. Inject token as environment variable
4. Execute command in `~/repo` directory
   - Default: non-interactive, stdout/stderr streamed via `limactl shell`
   - With `-i`: interactive, TTY attached via `limactl shell --tty`
5. Wait for command completion or Ctrl+C

**Outputs:**
- Command stdout/stderr streamed to terminal
- Exit code propagated

#### `isolarium destroy`

Deletes the VM and all its contents.

**Preconditions:**
- VM exists

**Actions:**
1. Stop VM if running
2. Delete VM via Lima

**Outputs:**
- VM removed
- All VM state (including cloned repo, credentials, logs) deleted

#### `isolarium status`

Reports current state.

**Outputs:**
- VM state (none, running, stopped)
- Associated repository (if VM exists)
- GitHub App configuration status (configured/not configured)

#### `isolarium config`

Manages GitHub App credentials in macOS Keychain.

**Subcommands:**
```
isolarium config set --app-id <id> --private-key-file <path>
isolarium config show
isolarium config delete
```

#### `isolarium shell`

Opens an interactive shell inside the VM.

**Preconditions:**
- VM exists and is running

---

## 5. VM Image Specification

### 5.1 Base Configuration

| Component | Specification |
|-----------|---------------|
| Platform | Linux (Lima on macOS Apple Silicon) |
| Base image | Ubuntu LTS or similar |
| Filesystem | Isolated; no host mounts |
| Network | Full internet access |

### 5.2 Pre-installed Tools

| Tool | Version/Source |
|------|----------------|
| git | Latest from package manager |
| Node.js | LTS version |
| Claude Code | Latest |
| Docker | Latest from package manager |
| GitHub CLI (`gh`) | Latest |
| JDK 17 | Installed via SDKMAN |
| SDKMAN | Latest |

### 5.3 Security Properties

- No host filesystem mounts
- No host Docker socket exposure
- No ambient git credentials
- No personal identity exposed (agent uses GitHub App identity)

---

## 6. Credential Flow

### 6.1 GitHub App Token Minting

```
┌─────────────────────────────────────────────────────────────────┐
│ Host (macOS)                                                    │
│                                                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────────┐ │
│  │ Keychain    │───▶│ Isolarium   │───▶│ GitHub API          │ │
│  │ (App ID +   │    │ CLI         │    │ (mint installation  │ │
│  │ private key)│    │             │◀───│  token)             │ │
│  └─────────────┘    └─────────────┘    └─────────────────────┘ │
│                            │                                    │
│                            │ inject token                       │
│                            ▼                                    │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Lima VM                                                  │   │
│  │                                                          │   │
│  │   GIT_TOKEN env var ──▶ git clone / push               │   │
│  │                                                          │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### 6.2 Token Properties

| Property | Value |
|----------|-------|
| Lifetime | Short (minutes to hours, per GitHub App configuration) |
| Scope | Single repository |
| Permissions | Contents (read/write), Pull Requests (read/write) |

### 6.3 Claude Code Session

| Mode | Behavior |
|------|----------|
| `--copy-session` (default) | Copy credentials file specified by `CLAUDE_CREDENTIALS_PATH` to VM `~/.claude/.credentials.json` with permissions `600`; no login required |
| `--fresh-login` | User authenticates via device code flow; separate session |

**Environment variable:** `CLAUDE_CREDENTIALS_PATH` - path to credentials file on host (required for `--copy-session`)

---

## 7. Non-functional Requirements

### 7.1 Performance

| Metric | Target |
|--------|--------|
| VM creation time | < 2 minutes (first time); < 30 seconds (cached image) |
| `isolarium run` startup overhead | < 10 seconds |
| Script I/O latency | Near real-time streaming |

### 7.2 Reliability

| Requirement | Description |
|-------------|-------------|
| Idempotent destroy | `isolarium destroy` succeeds even if VM is in unknown state |
| Graceful Ctrl+C | Script termination propagates cleanly |
| Token refresh | Fresh token minted on each `run` (no stale token issues) |

### 7.3 Usability

| Requirement | Description |
|-------------|-------------|
| Single binary | Distributed as standalone Go binary |
| Minimal configuration | Only GitHub App credentials required |
| Clear error messages | Actionable guidance for common failures |

### 7.4 Security

| Requirement | Description |
|-------------|-------------|
| Host isolation | VM cannot access host filesystem or credentials |
| Credential scoping | Tokens limited to single repository |
| Audit trail | All repository actions attributed to GitHub App |
| Disposability | Compromise recovery via VM destruction |

---

## 8. Scenarios and Workflows

### 8.1 Primary End-to-End Scenario: Run Agent on Repository

**Actors:** Developer, Claude Code, GitHub

**Preconditions:**
- Developer has GitHub App installed on repository
- Developer has configured `isolarium config` with App credentials
- Developer has Claude Code Max subscription authenticated on host

**Steps:**

1. Developer navigates to local git checkout: `cd ~/code/myproject`
2. Developer creates VM: `isolarium create`
   - CLI reads remote URL from `.git/config`
   - CLI retrieves App credentials from Keychain
   - CLI creates Lima VM
   - CLI mints installation token
   - CLI clones repo inside VM
3. Developer runs agent: `isolarium run -i -- claude`
   - CLI copies Claude session into VM
   - CLI mints fresh token
   - CLI executes command in `~/repo` with TTY attached
   - Claude Code runs, makes changes, commits, pushes to branch
5. Developer reviews changes on GitHub
6. Developer destroys VM: `isolarium destroy`

**Postconditions:**
- Agent's changes are on a branch in GitHub
- VM is deleted
- No credentials or code remain on developer machine outside normal host checkout

### 8.2 Scenario: Debug Inside VM

**Steps:**

1. Developer has an existing VM from `isolarium create`
2. Developer runs: `isolarium shell`
3. Interactive shell opens inside VM
4. Developer inspects files, runs commands, diagnoses issues
5. Developer exits shell
6. Developer continues with `isolarium run` or `isolarium destroy`

### 8.3 Scenario: Fresh Claude Login

**Steps:**

1. Developer runs: `isolarium run -i --fresh-login -- claude`
2. Claude Code inside VM displays device code URL
3. Developer opens URL in browser on host, enters code, authenticates
4. Claude Code receives session tokens
5. Command continues with Claude Code authenticated

### 8.4 Scenario: Recover from Suspected Compromise

**Steps:**

1. Developer observes suspicious agent behavior
2. Developer presses Ctrl+C to stop command
3. Developer runs: `isolarium destroy`
4. VM and all contents are deleted
5. Developer optionally rotates GitHub App credentials
6. Developer creates fresh VM: `isolarium create`

---

## 9. Constraints and Assumptions

### 9.1 Constraints

| Constraint | Description |
|------------|-------------|
| macOS only | Lima requires macOS; Keychain integration is macOS-specific |
| Apple Silicon | Optimized for Apple Silicon (ARM64 VMs) |
| Single VM | Initial version supports only one VM at a time |
| GitHub only | Repository access assumes GitHub; no GitLab/Bitbucket support |
| Claude Code only | Script is expected to use Claude Code; other agents not tested |

### 9.2 Assumptions

| Assumption | Description |
|------------|-------------|
| Lima installed | User has Lima installed (`brew install lima`) |
| GitHub App exists | User has already created and installed a GitHub App on their repo |
| Network available | VM requires internet for GitHub, Docker Hub, Anthropic API |
| Claude Code Max | User authenticates via OAuth, not API key |

### 9.3 Out of Scope (Initial Version)

- Multi-VM support
- Non-GitHub repositories
- Non-Claude-Code agents
- Network egress restrictions
- GitHub App creation wizard
- Windows or Linux host support

---

## 10. Acceptance Criteria

### 10.1 Core Functionality

| ID | Criterion |
|----|-----------|
| AC-1 | `isolarium config set` stores GitHub App credentials in macOS Keychain |
| AC-2 | `isolarium create` provisions a Lima VM with all required tools installed |
| AC-3 | `isolarium create` clones the repository inside the VM using a minted token, checking out the same branch as the host |
| AC-4 | `isolarium run -- command` executes the command inside the VM in the repo directory |
| AC-5 | `isolarium run` injects a fresh GitHub token as an environment variable |
| AC-6 | `isolarium run --copy-session` makes Claude Code work without login |
| AC-7 | `isolarium run --fresh-login` triggers device code authentication |
| AC-8 | `isolarium shell` opens an interactive shell in the VM |
| AC-9 | `isolarium destroy` deletes the VM completely |
| AC-10 | `isolarium status` reports VM state and configuration |

### 10.2 Security Properties

| ID | Criterion |
|----|-----------|
| AC-S1 | The VM cannot access host filesystem (no mounts) |
| AC-S2 | The VM cannot access host Docker socket |
| AC-S3 | GitHub tokens are scoped to a single repository |
| AC-S4 | GitHub tokens are short-lived (honor App configuration) |
| AC-S5 | No personal git credentials are present in the VM |

### 10.3 Usability

| ID | Criterion |
|----|-----------|
| AC-U1 | `isolarium create` completes in under 2 minutes on first run |
| AC-U2 | `isolarium run` startup overhead is under 10 seconds |
| AC-U3 | Ctrl+C during `run` terminates the command cleanly |
| AC-U4 | Error messages include actionable guidance |

---

## 11. Implementation Language and Distribution

| Aspect | Decision |
|--------|----------|
| Language | Go |
| CLI framework | Cobra (recommended) |
| GitHub API | go-github library |
| Distribution | Single binary, private initially, open source later |

---

## 12. Dependencies

| Dependency | Purpose |
|------------|---------|
| Lima | VM management |
| macOS Keychain | Credential storage |
| GitHub API | Token minting |
| Docker | Runs inside VM for agent tasks |

---

## 13. Glossary

| Term | Definition |
|------|------------|
| GitHub App | A GitHub identity for automation, separate from user accounts |
| Installation token | Short-lived token scoped to repos where the App is installed |
| Lima | Linux virtual machine manager for macOS |
| Device code flow | OAuth flow where user authenticates via browser using a code |
| Claude Code Max | Anthropic's subscription plan for Claude Code with OAuth authentication |

---

## Change History

### 2026-02-04: Branch handling clarification

Updated repository handling to specify that the VM clones/checks out the same branch as the host repo:
- Section 3.4: Repository Handling capabilities
- Section 4.2: `isolarium create` actions and outputs
- Section 10.1: Acceptance criteria AC-3

### 2026-02-05: Revised Claude credentials handling

Changed from copying entire `~/.claude/` directory to copying only the credentials file:
- New environment variable `CLAUDE_CREDENTIALS_PATH` specifies credentials file on host
- File is copied to VM at `~/.claude/.credentials.json` with permissions 600
- Updated sections 3.3, 4.2, and 6.3

### 2026-02-06: Revised `run` command from script execution to command execution

Changed `run` from copying and executing a host script to running a command directly inside the VM:
- New syntax: `isolarium run [options] -- command arg...`
- Commands run in `~/repo` directory inside VM — no files copied from host
- Non-interactive by default (stdout/stderr streamed)
- New `--interactive`/`-i` flag attaches TTY via `limactl shell --tty` for interactive commands
- Removed `--script` flag
- Updated sections 3.5, 4.1, 4.2, 8.1, 8.3, 8.4, 10.1, 10.3
