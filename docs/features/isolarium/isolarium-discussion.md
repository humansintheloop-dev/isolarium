# Isolarium Discussion

## Classification
**C. Platform/infrastructure capability**

Isolarium is developer tooling that provides a secure execution environment for autonomous coding agents. It is not a user-facing feature (no end-users beyond the developer), not an architecture POC (the architecture is defined), and not an educational example. It is infrastructure that enables a secure agent execution workflow.

## Questions and Answers

### Q1: What form should this software take?

The idea document describes the architecture and security model, but not the software artifact itself. What should "Isolarium" be?

**Options:**
A. **CLI tool** - A command-line tool that manages the VM lifecycle and credential injection (e.g., `isolarium run --repo owner/repo`)
B. **Shell scripts + documentation** - A collection of scripts and guides that users customize for their workflow
C. **Configuration-as-code framework** - Declarative configuration files (YAML/TOML) that define agent environments, with a runtime to execute them
D. **Library/SDK** - A programmatic interface for building custom agent execution pipelines

**Answer:** A - CLI tool

---

### Q2: What coding agents should this support initially?

Different agents have different requirements for how they're invoked and what environment they expect.

**Options:**
A. **Claude Code only** - Focus on Claude Code's specific requirements (Node.js runtime, API key injection)
B. **Claude Code + Aider** - Support the two most popular CLI-based coding agents
C. **Agent-agnostic** - Design for arbitrary commands; the user specifies what to run inside the VM
D. **Pluggable architecture** - Built-in support for common agents, with an extension mechanism for others

**Answer:** A - Claude Code only (focused initial scope)

---

### Q3: What is the expected VM lifecycle model?

How should the CLI manage VM creation and destruction?

**Options:**
A. **Ephemeral per-run** - Create a fresh VM for each `isolarium run`, destroy it when the agent exits
B. **Session-based** - Create a VM that persists for a working session; explicit `isolarium destroy` to clean up
C. **Long-lived with reset** - Keep a single VM running; `isolarium reset` to wipe it to a clean state
D. **User-managed** - CLI assumes a Lima VM already exists; user handles VM lifecycle separately

**Answer:** B - Session-based (VM persists for working session, explicit destroy to clean up)

---

### Q4: How should GitHub App credentials be managed?

The idea document specifies GitHub App installation tokens for repo-scoped access. How should the CLI obtain these?

**Options:**
A. **CLI mints tokens directly** - User provides App ID + private key; CLI generates installation tokens on demand
B. **External token provider** - User runs a separate process or script that provides tokens; CLI consumes them
C. **Interactive OAuth flow** - CLI guides user through a one-time setup, then manages token refresh automatically
D. **Environment variable injection** - User is responsible for obtaining tokens; CLI just passes them into the VM

**Answer:** A - CLI mints tokens directly (user provides App ID + private key)

---

### Q5: Where should GitHub App credentials (App ID + private key) be stored?

The private key is sensitive and long-lived. Where should it reside on the host?

**Options:**
A. **macOS Keychain** - Store in the system keychain; CLI retrieves at runtime (most secure, macOS-specific)
B. **Config file with restricted permissions** - Store in `~/.config/isolarium/` with 600 permissions (portable, user responsibility to protect)
C. **Environment variables only** - Never persist to disk; user sets `ISOLARIUM_APP_ID` and `ISOLARIUM_PRIVATE_KEY` each session
D. **1Password / external secret manager** - Integrate with `op` CLI or similar; user configures retrieval command

**Clarification requested:** How is the keychain accessed? Is it synchronized across Macs?
- Access via `security` CLI tool; may prompt for password or Touch ID
- If iCloud Keychain is enabled, items sync across all Macs with the same Apple ID
- Can also use local-only keychain for single-machine containment

**Answer:** A - macOS Keychain (synced via iCloud Keychain for convenience across machines)

---

### Q6: How should the Anthropic API key be handled?

Claude Code requires an Anthropic API key. Should this follow the same pattern as GitHub credentials?

**Options:**
A. **Same as GitHub credentials** - Store in macOS Keychain alongside the GitHub App private key
B. **Environment variable passthrough** - Expect `ANTHROPIC_API_KEY` to be set on the host; pass it into the VM
C. **Prompt at runtime** - Ask for the API key each time (most secure, least convenient)
D. **Separate configuration** - Let user choose per-credential (some in keychain, some in env vars)

**Answer:** User has Claude Code Max subscription (OAuth-based, not API key)

---

### Q6 (revised): How should Claude Code Max authentication work inside the VM?

**Clarification requested:** How does auth work without a browser in the VM?
- Claude Code supports device code flow: shows URL+code, user authenticates via any browser

**Clarification requested:** What does "copy session from host" mean?
- Claude Code stores OAuth tokens in `~/.claude/` on host
- "Copy" means CLI copies those files into VM before starting
- Tradeoff: zero friction, but exposes host session to VM

**User observation:** If VM is compromised, it always has session tokens (regardless of how auth happened)
- Correct. The difference is only revocation granularity and audit clarity, not security posture while running.

**Answer:** C - User's choice at runtime (`--copy-session` as default vs `--fresh-login` for separate session)

---

### Q7: How should the target repository be specified?

The CLI needs to know which repo to clone and which GitHub App installation to use for tokens.

**Options:**
A. **Command-line argument** - `isolarium run owner/repo` (explicit each time)
B. **Config file per-repo** - `.isolarium.yaml` in the repo itself (requires initial clone to read)
C. **Host directory detection** - Run from a git checkout on host; CLI reads remote URL (convenient but ties to host state)
D. **Named profiles** - `isolarium run --profile myproject` where profiles are defined in `~/.config/isolarium/profiles.yaml`

**Answer:** C - Host directory detection (run from git checkout on host; CLI reads remote URL)

---

### Q8: How should the agent's changes get back to you?

The agent works inside the VM on a separate clone. How do its changes reach your host or GitHub?

**Options:**
A. **Push to remote only** - Agent commits and pushes to a branch on GitHub; you pull on host (clean separation, requires network)
B. **Copy files to host** - CLI copies changed files from VM back to host working directory (convenient, but mixes VM output with host state)
C. **Push to remote, then auto-pull on host** - Agent pushes, CLI automatically pulls the branch to your host checkout
D. **User decides per-run** - `--push` (remote only) vs `--sync` (copy back to host)

**Answer:** Not applicable as framed. Isolarium runs a user-provided script inside the VM; that script is responsible for invoking Claude Code and handling git operations (push, PR creation, etc.). Isolarium is an execution environment manager, not an agent orchestrator.

---

### Q9: How should the user-provided script be specified?

Isolarium sets up the environment, then runs a script that handles the actual agent invocation and git workflow.

**Options:**
A. **Script in repo** - Convention-based: look for `.isolarium/run.sh` in the cloned repo
B. **Script path argument** - `isolarium run --script ./my-agent-workflow.sh` (passed from host, copied into VM)
C. **Inline command** - `isolarium run -- "claude -p 'Fix the bug' && git push"` (ad-hoc commands)
D. **Combination** - Default to repo script if present, allow override via `--script` or `--` for inline

**Answer:** B - Script path argument (`isolarium run --script ./my-agent-workflow.sh`) for initial simplicity

---

### Q10: What should be pre-installed in the VM image?

The VM needs certain tools to be useful. What should the base image include?

**Options:**
A. **Minimal + Claude Code only** - Base Linux, git, Node.js, Claude Code; user's script installs anything else
B. **Common dev tools** - Add Docker, Python, common build tools (gcc, make); covers most repos
C. **Configurable layers** - Base image + optional "flavors" (`--flavor python`, `--flavor node`, `--flavor java`)
D. **Full dev environment** - Kitchen sink: Docker, multiple language runtimes, common databases; larger image but ready for anything

**Answer:** Specific requirements:
- Base Linux + git + Node.js + Claude Code
- Docker
- GitHub CLI (`gh`)
- JDK 17 (installed via SDKMAN)

---

### Q11: Should the VM have network access restrictions?

The idea doc mentions isolation, but network access is needed for git, Docker pulls, and API calls. Should there be restrictions?

**Options:**
A. **Full network access** - VM can reach any host (simplest, matches typical dev environment)
B. **Allowlist only** - Restrict to known-good hosts: GitHub, Anthropic API, Maven Central, Docker Hub, etc.
C. **Egress logging** - Full access but log all outbound connections for audit
D. **Air-gapped option** - Support `--offline` mode for sensitive work (pre-pull dependencies)

**Answer:** A - Full network access (simplest, matches typical dev environment)

---

### Q12: What language should the CLI be implemented in?

The CLI needs to interact with Lima, macOS Keychain, GitHub API, and the filesystem.

**Options:**
A. **Go** - Single binary, good CLI tooling (cobra), easy cross-compile, strong GitHub API libraries
B. **Rust** - Single binary, excellent performance, strong typing, good CLI ecosystem (clap)
C. **Python** - Rapid development, good libraries, but requires Python runtime on host
D. **Shell scripts** - Minimal dependencies, but harder to maintain as complexity grows

**Answer:** A - Go (single binary, good CLI tooling, strong GitHub API libraries)

---

### Q13: What CLI subcommands should exist?

Based on the session-based VM lifecycle and script execution model, what commands are needed?

**Proposed command structure:**
```
isolarium create          # Create and start a new VM for current repo
isolarium run --script x  # Run script in existing VM
isolarium destroy         # Delete the VM
isolarium status          # Show VM state, repo, credentials status
isolarium config          # Manage GitHub App credentials in Keychain
```

**Options:**
A. **Accept this structure** - These commands cover the workflow
B. **Combine create+run** - `isolarium run` creates VM if needed, then runs script
C. **Add more commands** - Suggest additional commands needed
D. **Simplify further** - Fewer commands, more flags

**Answer:** A - Accept this structure (create, run, destroy, status, config)

---

### Q14: How should GitHub App setup work?

The user needs a GitHub App installed on their repo before isolarium can mint tokens. Should isolarium help with this?

**Options:**
A. **Documentation only** - Provide a guide; user creates App manually via GitHub UI
B. **CLI-assisted creation** - `isolarium init` walks user through creating a GitHub App via API, stores credentials
C. **Manifest flow** - `isolarium init` uses GitHub's App Manifest flow to create a pre-configured App with correct permissions
D. **Assume existing App** - User already has a GitHub App; isolarium only handles credential storage and token minting

**Answer:** D - Assume existing App (user creates App manually; isolarium handles credential storage and token minting only)

---

### Q15: Can multiple VMs exist simultaneously?

You might work on multiple repositories. Should isolarium support multiple concurrent VMs?

**Options:**
A. **Single VM only** - One VM at a time; `isolarium create` fails if a VM already exists
B. **Named VMs** - `isolarium create --name project-a`; multiple VMs can coexist, addressed by name
C. **Auto-named by repo** - VM name derived from repo (e.g., `owner-repo`); automatic multi-repo support
D. **Single VM initially** - Start with single VM; multi-VM is a future enhancement

**Answer:** D - Single VM initially (multi-VM is a future enhancement)

---

### Q16: How should `isolarium run` interact with the script?

When the user runs `isolarium run --script ./agent.sh`, how should the CLI handle I/O and lifecycle?

**Options:**
A. **Attached** - Script's stdout/stderr streams to terminal; CLI waits for completion; Ctrl+C stops script
B. **Detached with logs** - Script runs in background; `isolarium logs` to view output; `isolarium stop` to terminate
C. **Attached by default, detachable** - Default attached; `--detach` flag for background execution
D. **SSH session** - Drop user into SSH session inside VM where script runs; full interactive control

**Answer:** A - Attached (stdout/stderr streams to terminal; CLI waits for completion; Ctrl+C stops script)

---

### Q17: Should there be a way to SSH into the VM for debugging?

Sometimes you'll need to inspect the VM state, debug issues, or run ad-hoc commands.

**Options:**
A. **Yes, explicit command** - `isolarium ssh` opens an interactive shell in the VM
B. **Via Lima directly** - Document that users can use `limactl shell <vm-name>` (no extra command needed)
C. **Both** - `isolarium ssh` as convenience wrapper, document Lima fallback
D. **No** - Keep VM opaque; if something's wrong, destroy and recreate

**Answer:** A - Yes, explicit command (`isolarium ssh` opens an interactive shell in the VM)

---

### Q18: Should this be open source?

This affects documentation style, contribution model, and whether others can use it.

**Options:**
A. **Yes, from the start** - Public repo, open source license (MIT/Apache)
B. **Private initially, open later** - Develop privately, open source once stable
C. **Private/internal only** - Personal tooling, no plans to share
D. **Undecided** - Decide later based on how it evolves

**Answer:** B - Private initially, open later (develop privately, open source once stable)

---

### Q19: What's the primary repo you'll test this with?

Understanding the target repo helps ensure the VM image and workflow fit real needs. What kind of project will you run agents on?

**Options:**
A. **Java/Gradle project** - Aligns with JDK 17 requirement; likely needs Docker for integration tests
B. **Multi-module monorepo** - More complex build, longer clone times
C. **Simple test repo** - Create a minimal repo specifically for testing isolarium
D. **Other** - Describe your target

**Answer:** (Withdrawn - not relevant to design)

---

## Summary of Decisions

| Aspect | Decision |
|--------|----------|
| Form | CLI tool |
| Agent support | Claude Code only (initial) |
| VM lifecycle | Session-based (explicit create/destroy) |
| GitHub credentials | CLI mints tokens; App ID + private key in macOS Keychain (synced) |
| Claude Code auth | User's choice: `--copy-session` (default) vs `--fresh-login` |
| Repo specification | Detect from host's current git directory |
| Script handling | User script handles Claude invocation and git operations |
| Script specification | `--script` argument |
| VM image contents | Base Linux, git, Node.js, Claude Code, Docker, GitHub CLI, JDK 17 (SDKMAN) |
| Network access | Full (no restrictions) |
| Implementation language | Go |
| CLI commands | `create`, `run`, `destroy`, `status`, `config`, `ssh` |
| GitHub App setup | Assume existing App; user creates manually |
| Multi-VM support | Single VM initially |
| Script I/O | Attached (streams to terminal, Ctrl+C stops) |
| Open source | Private initially, open later |

---

### Classification

**Finalized:** C. Platform/infrastructure capability

See `isolarium-spec.md` for the complete specification.
