# Support Nono - Discussion

## Context (derived from codebase analysis)

- **Isolarium** is a Go CLI that provides isolated execution environments for AI coding agents (primarily Claude Code).
- **Current isolation**: Lima VMs only (full Linux VM on macOS, strong isolation, no host filesystem mounts).
- **Implemented isolation**: Container-based (Docker) - fully implemented with Backend interface, `--type` flag, auto-detection, and `(name, type)` identity pairs.
- **Nono** is a capability-based sandbox for AI agents ("the opposite of YOLO"). It provides process-level sandboxing on the host OS with fine-grained filesystem, network, and command restrictions. It is fundamentally different from VM/container isolation - lightweight, runs directly on macOS, no separate kernel or filesystem.

### Nono capabilities (from `nono run --help`):
- `--allow`, `--read`, `--write` for directory-level access control
- `--allow-file`, `--read-file`, `--write-file` for file-level access
- `--net-block` to disable network
- `--allow-command` / `--block-command` for command restrictions
- `--profile` for named configuration profiles
- `--secrets` for keystore-based secret injection
- `--exec` for TTY preservation (interactive apps like Claude Code)
- `--dry-run` for previewing sandbox configuration

### Nono-playground examples show:
- Python/pytest with uv (directory + cache access)
- Java/Gradle builds (read-only SDK access, writable build dirs)
- Claude CLI (fine-grained Claude config file access, keychain read)

---

## Questions & Answers

### Q1: How does nono fit into isolarium?

**Options:**
- A. As a third isolation backend alongside VM and container (same Backend interface, `--type nono`)
- B. As a complementary layer wrapping commands inside VMs/containers
- C. As a lightweight alternative for local development only
- D. Something else

**Answer:** A - As a third isolation backend implementing the same Backend interface.

**Implications:** Nono becomes a `--type` option. The planned Backend interface must accommodate nono's host-level sandboxing model, which differs significantly from VM/container approaches (no separate filesystem, no "create" step in the traditional sense, capability-based permissions instead of full isolation).

---

### Q2: How should the nono backend handle the `create`/`destroy` lifecycle?

**Options:**
- A. No-op create/destroy (just validate nono is installed, metadata only)
- B. Profile-based create (generate a nono profile in `~/.config/nono/profiles/`, `run` uses `--profile`, `destroy` removes it)
- C. Minimal state create (clone repo to sandboxed working directory)
- D. Something else

**Answer:** B - Profile-based create. `isolarium create --type nono` generates a nono profile tailored to the project. `isolarium run` uses `--profile` to reference it. `isolarium destroy` removes the profile and metadata.

**Revised:** Defer profile generation for now. The nono backend will pass all options on the command line. `isolarium create --type nono` records metadata only (validates nono is installed, records project info). Profile-based configuration can be added later.

**User clarification:** Nono profiles should live in the isolarium metadata directory (`~/.isolarium/{name}/nono/`) when eventually implemented. For now, everything is command-line flags.

---

### Q3: What is the working directory model for the nono backend?

**Options:**
- A. Current directory (user's CWD, agent works on local files directly)
- B. Explicit project directory specified at create time
- C. Cloned repo to separate location

**Answer:** The CWD when isolarium is invoked is either (1) a regular repo or (2) a git worktree directory. The nono backend works directly on this directory - no cloning or separate workspace needed.

**Implications:** The nono backend uses the CWD as the sandboxed workspace. This is fundamentally different from the VM backend (which clones into the VM). The agent edits local files directly, with nono restricting access to only what's needed. Changes are immediately visible to the user (no sync step).

---

### Q4: How should credentials be handled for the nono backend?

**Options:**
- A. Grant read access to existing host credentials via nono's `--read-file` / `--read` flags
- B. Use nono's `--secrets` flag for keystore-based secret injection
- C. Same as VM backend (mint tokens, copy credentials)
- D. Hybrid (read Claude creds from host, mint GitHub tokens)

**Answer:** A - Grant read access to existing host credentials. The sandboxed process reads the user's existing Claude config, keychain, etc. directly from the host via nono's filesystem permission flags.

**Implications:** No credential copying or token minting needed for the nono backend. Simplifies the implementation significantly. The nono command line will include flags like `--read ~/.claude/`, `--read-file ~/Library/Keychains/login.keychain-db`, etc. The specific paths needed depend on what the sandboxed command requires (e.g., Claude Code needs different paths than a plain `git` command).

---

### Q5: Should isolarium hardcode permissions or let the user specify them?

**Options:**
- A. Hardcode for a known primary use case
- B. User-specified permissions at create time
- C. Convention-based defaults with overrides

**Answer:** A - Hardcode for Claude Code + Python. The nono backend ships with a built-in permission set that covers running Claude Code with Python tooling (uv, pytest, etc.).

**Implications:** The initial implementation targets a specific, well-defined use case: Claude Code working on a Python project. The permission set is derived from the nono-playground examples (`try-nono-claude.sh` + `try-nono.sh`). Additional permission sets (e.g., Java/Gradle) can be added later. This keeps the first implementation simple and testable.

---

### Q6: Should `isolarium run` with nono always run Claude Code, or wrap arbitrary commands?

**Options:**
- A. Always Claude Code (no command argument needed)
- B. Wrap arbitrary commands with the hardcoded permission set
- C. Default to Claude, allow override

**Answer:** B - Wrap arbitrary commands. `isolarium run --type nono -- <command>` wraps any user-specified command with the hardcoded Claude+Python permission set. The user can run `claude`, `pytest`, `git`, etc.

**Implications:** The nono backend applies the same permission set regardless of the command. This is consistent with how the existing VM backend works (`isolarium run -- <command>`). The permission set is broad enough for Claude Code + Python development, so simpler commands like `pytest` or `git` will work within those same permissions.

---

### Q7: How should TTY/interactive mode work with nono?

**Options:**
- A. Always enable nono's `--exec` flag (TTY passthrough always on)
- B. Map isolarium's `--interactive` / `-i` flag to nono's `--exec` flag

**Answer:** B - Map to `--interactive`. `isolarium run -i --type nono -- claude` passes `--exec` to nono for TTY preservation. Without `-i`, nono monitors output normally (useful for non-interactive commands like `pytest`).

**Implications:** Consistent with the existing VM backend's `-i` flag behavior. Users run `isolarium run -i --type nono -- claude` for interactive Claude sessions, and `isolarium run --type nono -- pytest` for non-interactive commands.

---

### Q8: Should `isolarium shell --type nono` be supported?

**Options:**
- A. Map to `nono shell` with the same permission set
- B. Not supported (doesn't make sense for host-level sandboxing)
- C. Defer for initial implementation

**Answer:** A - Map to `nono shell`. `isolarium shell --type nono` runs `nono shell` with the hardcoded Claude+Python permission set, giving the user a sandboxed interactive shell in the project directory.

**Implications:** Provides a consistent command surface across all backends. Users can drop into a sandboxed shell to explore or debug, just as they would SSH into a VM.

---

### Q9: Should the Backend interface be introduced as part of this feature?

**Context update:** The container-based isolation PR has been merged. The Backend interface already exists at `internal/backend/backend.go`:
```go
type Backend interface {
    Create(name string, opts CreateOptions) error
    Destroy(name string) error
    Exec(name string, envVars map[string]string, args []string) (int, error)
    ExecInteractive(name string, envVars map[string]string, args []string) (int, error)
    OpenShell(name string, envVars map[string]string) (int, error)
    GetState(name string) string
    CopyCredentials(name string, credentials string) error
}
```

Backend resolution (`ResolveBackend`), `--type` flag, auto-detection, and `(name, type)` identity pairs are all in place. Lima and Docker backends are implemented. Adding nono means implementing a third backend following established patterns.

**Answer:** No longer a question - the Backend interface exists. Nono will be the third implementation.

---

### Q10: What should `isolarium status --type nono` report?

**Options:**
- A. Metadata only (environment exists, project directory, creation time). State is always "configured" since there's no persistent process.
- B. Metadata plus check whether `nono` is installed and functional on the host.
- C. Something else.

**Answer:** A - Metadata only. Report that the nono environment exists, the project directory, and when it was created. State is always "configured" (no running/stopped concept).

**Implications:** `GetState()` for the nono backend returns a fixed value like `"configured"` rather than `"running"`/`"stopped"`. The status display will show nono-specific fields (project directory) similar to how containers show work directory.

---

### Derived: CopyCredentials is a no-op for nono

Since the nono sandbox reads host credentials directly via filesystem permission flags (Q4), there's nothing to copy. `CopyCredentials()` returns nil. Credential access is handled entirely by the nono flags that `Exec`/`ExecInteractive`/`OpenShell` construct.

---

### Q11: How should inapplicable flags (`--copy-session`, `--fresh-login`) behave with nono?

**Options:**
- A. Error - reject flags that don't apply to the nono backend
- B. Silently ignore
- C. Warning but proceed

**Answer:** A - Error. Reject flags that don't apply to the nono backend, making it clear the user is using an incorrect combination.

**Implications:** The CLI `run` command validates flag combinations against the backend type before executing. Similar to how `--work-directory` is rejected for the VM backend in the existing code.

---

### Q12: Should the nono backend allow or block network access?

**Options:**
- A. Allow network (nono's default) - needed for Claude API, package downloads, git
- B. Block network (`--net-block`) - maximum isolation but breaks most workflows
- C. Configurable with allow as default

**Answer:** A - Allow network (nono's default). Claude Code needs the Anthropic API, Python tooling needs package downloads, and git needs remote access.

**Implications:** No `--net-block` flag passed to nono. Network is allowed by default in nono, so no extra flags needed.

---

### Q13: What should the default environment name be for nono?

**Options:**
- A. `"isolarium-nono"` - follows the container pattern (`isolarium-{type}`)
- B. `"isolarium"` - same as VM, distinguished by `(name, type)` pair
- C. Something else

**Answer:** A - `"isolarium-nono"`. Follows the established pattern of `isolarium-{type}`.

---

### Q14: Should the nono backend customize nono's default command blocklist?

Nono blocks destructive commands like `rm`, `dd`, `chmod` by default. Claude Code may need some of these.

**Options:**
- A. Use nono's defaults - let the built-in blocklist stand as intended safety behavior
- B. Allow `rm` only (`--allow-command rm`)
- C. Allow all commonly needed commands (`rm`, `chmod`, etc.)

**Answer:** A - Use nono's defaults. If Claude Code hits a blocked command, that's the intended safety behavior.

**Implications:** No `--allow-command` or `--block-command` flags passed. Nono's built-in safety defaults are appropriate for sandboxing AI agents (that's nono's primary purpose).

---

### Derived: VM-specific commands don't apply to nono

`clone-repo`, `install-tools`, and `install-workflow-tools-from-source` are VM-specific (they clone/install into a VM). Since the nono backend works directly on local files with no separate environment to provision, these commands should error if invoked with `--type nono`.

---

## Classification

**Type:** A - User-facing feature

**Rationale:** This adds a new isolation backend that users select via `--type nono`. It extends the existing Backend interface with a third implementation, exposes new behavior through existing CLI commands (`create`, `run`, `ssh`, `destroy`, `status`), and targets a specific user workflow (running Claude Code with Python on a local project with lightweight sandboxing). It is not an architecture POC (the Backend interface is already proven with two implementations), not platform/infrastructure (it's a user-selectable feature), and not educational/example.

---

