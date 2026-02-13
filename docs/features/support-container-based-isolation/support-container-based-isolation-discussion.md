# Support Container-Based Isolation - Discussion

## Classification

**A. User-facing feature**

**Rationale:** This adds a new `--type container` option to the existing isolarium CLI, with a new container backend (Dockerfile, Docker lifecycle management, credential injection via `gh auth token`). It extends the user-facing command set (`create`, `run`, `shell`, `destroy`, `status`) with a second isolation strategy. It is not an architecture POC (the intent is to ship it), not infrastructure/platform (it's a feature of the developer-facing CLI), and not educational.

## Questions and Answers

### Q1: What is the primary motivation for adding container-based isolation?

Currently, isolarium uses Lima VMs which provide strong kernel-level isolation (the security model states "VM compromise is acceptable; host compromise is not"). Containers share the host kernel, which is a weaker isolation boundary.

The idea also introduces `--work-directory` to mount a host directory as `~/repo`, which is another departure from the current model where the repo is cloned *inside* the VM with no host mounts.

**What is the primary motivation for adding container support?**

- A. **Speed/convenience** — VMs are slow to create; containers start in seconds, useful for quick tasks or iteration
- B. **Linux/CI support** — Lima is macOS-only; containers would let isolarium run on Linux and in CI pipelines
- C. **Local development workflow** — developers want to edit files on the host and run agents against them in a container (the `--work-directory` mount)
- D. **All of the above / other**

**Answer:** D — All of the above. Speed/convenience, Linux/CI support (Lima is macOS-only), and local development workflow with host directory mounts are all motivating factors.

### Q2: What is the acceptable security model for container-based isolation?

VM isolation protects against host compromise even if the agent is malicious (separate kernel, no host mounts). Containers are weaker: they share the host kernel, and with `--work-directory` the agent has direct access to host files.

**How should we think about security for the container mode?**

- A. **Trusted-agent model** — Container mode is for trusted agents/code only. The isolation is about environment consistency (dependencies, tools), not security. Document this clearly and don't invest in hardening.
- B. **Best-effort hardening** — Use rootless Docker, read-only root filesystem, drop capabilities, restrict network — but accept that container isolation is weaker than VM. Clearly document the trade-off.
- C. **Equivalent security required** — Container mode should achieve VM-equivalent isolation (e.g., using gVisor/kata containers). If we can't, don't ship it.

**Answer:** B — Best-effort hardening. Use rootless Docker, drop all capabilities, no-new-privileges, scope the mount to just the work directory. Accept that container isolation is weaker than VM and document the trade-off clearly. Container mode provides environment isolation but not security isolation; for untrusted workloads, use VM mode.

### Q3: Which existing CLI commands should support the `--type container` flag?

Currently isolarium has these commands: `create`, `run`, `shell` (ssh), `destroy`, `status`, `clone-repo`, `install-tools`.

With container mode, some commands map naturally:
- `create` → build image + start container (instead of Lima VM)
- `run` → `docker exec` (instead of `limactl shell`)
- `shell`/`ssh` → `docker exec -it bash` (instead of `limactl shell --tty`)
- `destroy` → `docker rm -f` + optionally remove image (instead of `limactl delete`)
- `status` → `docker inspect` (instead of Lima state check)

Some commands may not apply:
- `clone-repo` — with `--work-directory`, the repo is mounted from the host, not cloned inside. This command may not be needed for container mode.
- `install-tools` — tools would be baked into the Dockerfile rather than installed post-creation.

**Should all commands support `--type`, or should we start with a minimal set?**

- A. **Minimal set** — `create`, `run`, `shell`, `destroy`, `status` only. Skip `clone-repo` and `install-tools` for container mode since they don't apply (repo is mounted, tools are in the image).
- B. **All commands** — Make every command work with both types for consistency, adapting behavior as needed (e.g., `clone-repo` could copy files into the container).
- C. **Even more minimal** — Start with just `create`, `shell`, and `destroy` to validate the approach, add others later.

**Answer:** A — Minimal set: `create`, `run`, `shell`, `destroy`, `status`. `clone-repo` and `install-tools` don't apply to container mode since the repo is mounted from the host and tools are baked into the Dockerfile.

### Q4: How should the `--type` flag and `--work-directory` be persisted and resolved?

Currently, the VM name is the key identifier (default: "isolarium"), and metadata is stored at `~/.isolarium/{vm-name}/repo.json`. With container mode, we need to track which type an environment is and what work directory it uses.

A few decisions here:

**Can a user have both a VM and a container environment at the same time?**

- A. **No, one at a time** — `--type` selects the isolation strategy for the single "isolarium" environment. Running `create --type container` when a VM exists would error (must `destroy` first).
- B. **Yes, separate by name** — The default name remains "isolarium" for VMs. Containers get a different default name (e.g., "isolarium-container") or the user specifies `--name`. Each name maps to exactly one environment (VM or container).
- C. **Yes, type is part of the identity** — The environment is keyed by (name, type) pair. You could have an "isolarium" VM and an "isolarium" container simultaneously.

**Answer:** C — Type is part of the identity. The environment is keyed by (name, type) pair. A user could have an "isolarium" VM and an "isolarium" container simultaneously. Metadata storage would expand to something like `~/.isolarium/{name}/{type}/`.

### Q5: Where does the Dockerfile live and how is it managed?

In VM mode, `template.yaml` is embedded in the Go binary and defines the full VM configuration (OS image, CPU/memory, provisioning scripts for Node.js, Java, Docker, Claude Code, etc.).

For container mode, a Dockerfile serves a similar role. Several questions:

**Where should the Dockerfile come from?**

- A. **Embedded in the binary** — Ship a default Dockerfile (like template.yaml), built into the Go binary. The user gets a working container out of the box with the same tools as the VM (Node.js, git, Claude Code, etc.). No Dockerfile needed in the project.
- B. **Project-local Dockerfile** — The user provides a `Dockerfile.isolarium` (or similar) in their repo. This gives full control over the environment but requires setup per project.
- C. **Embedded default + project override** — Ship a default Dockerfile, but if the user has a project-local one (e.g., `Dockerfile.isolarium` or a path specified via flag), use that instead. Best of both worlds.

**Answer:** A — Embedded in the binary, like template.yaml. Ship a default Dockerfile with the same toolset (Node.js, git, Claude Code, etc.) so it works out of the box. No project-local Dockerfile needed initially.

### Q6: What tools should be pre-installed in the default container image?

The current VM template installs: git, curl, wget, Node.js LTS, GitHub CLI, Docker (rootless), Claude Code (npm), uv (Python), Java 17, Gradle 8.14, and SDKMAN.

For a container image, some of these don't apply or would be unusual:
- **Docker-in-Docker**: Running Docker inside a container adds complexity (needs privileged mode or DinD sidecar), which conflicts with the best-effort hardening decision.
- **Java/Gradle**: These are project-specific; they bloat the image for projects that don't need them.

**Which toolset should the default container image include?**

- A. **Minimal agent toolset** — git, curl, Node.js LTS, Claude Code, GitHub CLI, uv. Skip Docker-in-Docker and Java/Gradle. Keep the image lean; users who need more can request a project-override Dockerfile later (even though that's not in scope now).
- B. **Match the VM** — Include everything the VM has (except Docker-in-Docker). Larger image but consistent behavior between VM and container modes.
- C. **Layered approach** — Base image with minimal tools, plus optional "profiles" or build args to add Java/Gradle. More complex to implement but flexible.

**Answer:** Include everything from the VM except Docker-in-Docker. Specifically: git, curl, wget, Node.js LTS, GitHub CLI, Claude Code (npm), uv, SDKMAN, Java 17, Gradle 8.14. The Dockerfile should replicate the SDKMAN installation steps from `internal/lima/install-using-sdkman.sh`. No Docker-in-Docker.

### Q7: How should credentials be handled in container mode?

In VM mode, credentials work like this:
- **GitHub tokens**: Short-lived installation tokens minted from a GitHub App, injected as environment variables (`GIT_TOKEN`, `GH_TOKEN`) on each `run`/`shell` invocation.
- **Claude credentials**: Copied from host macOS Keychain to `~/.claude/.credentials.json` inside the VM (with `--copy-session` flag).

For container mode, the same mechanisms could work via `docker exec -e VAR=val` for env vars and `docker cp` for credential files.

**Should credential handling work the same way in container mode?**

- A. **Yes, same approach** — Mint GitHub App tokens and inject via env vars on `docker exec`. Copy Claude credentials file into container. Same `--copy-session` and `--fresh-login` flags.
- B. **Simplified for containers** — Since container mode targets trusted/local workflows, just pass through the host's existing git credentials and Claude session. Less secure but simpler.
- C. **Same approach but with a concern** — Same as A, but worth noting any concerns about credential handling differences.

**Answer:** Use the developer's existing `gh auth` token. Extract from host using `gh auth token`, inject as `GH_TOKEN` env var into the container via `docker exec -e`. The Dockerfile configures git to use `gh auth git-credential` as the credential helper, which reads `GH_TOKEN`. Both `gh` CLI and git HTTPS operations work with this single token. No GitHub App required for container mode. Note: `gh` stores tokens in macOS Keychain by default (not `hosts.yml`), so `gh auth token` is the reliable extraction method.

### Q8: How should Claude credentials be handled in container mode?

Separately from git/GitHub, the agent needs Claude API access. In VM mode, Claude credentials are read from the macOS Keychain on the host and copied to `~/.claude/.credentials.json` inside the VM (`--copy-session` flag).

For container mode, following the same simplified philosophy as git credentials:

- A. **Same copy approach** — Read credentials from Keychain on host, write to `~/.claude/.credentials.json` inside the container (via `docker cp` or volume mount). Keep `--copy-session` flag.
- B. **Mount the host's `~/.claude/` directory** — Bind-mount it read-only into the container. No copy step, always in sync. Simpler but gives the container access to all Claude config files, not just credentials.
- C. **Mount just the credentials file** — Bind-mount only `~/.claude/.credentials.json` read-only. Scoped access, no copy step.

**Answer:** A — Same copy approach, because it's the only viable option. Claude credentials are stored in the macOS Keychain (not as a file on disk), so mount-based options don't apply. The container mode will: (1) read credentials from Keychain using `security find-generic-password -s "Claude Code-credentials"`, (2) write into the container via `docker exec ... cat >` or `docker cp`, (3) set permissions to 600. Same `--copy-session` flag as VM mode. This reuses the existing `claude.ReadCredentialsFromKeychain()` function.

### Q9: How should the container lifecycle work?

In VM mode, `create` provisions a full VM (slow but persistent), and subsequent `run`/`shell` commands reuse it. The VM survives reboots.

For container mode with `--work-directory` (host mount), the container is more lightweight. Key lifecycle questions:

**What happens on `create`?**

- A. **Build image + start container** — `create` builds the Docker image (if not cached) and starts a long-running container (e.g., `docker run -d --name ... sleep infinity`). Subsequent `run`/`shell` use `docker exec`. The container persists until `destroy`.
- B. **Build image only** — `create` only builds the image. Each `run`/`shell` starts a fresh container (`docker run`) and removes it when done. No persistent container state.
- C. **Build image + start container, but ephemeral on shell exit** — `create` builds and starts, but the container is automatically removed when the last shell exits. Next `run`/`shell` creates a new one from the cached image.

**Answer:** A — Build image + start long-running container. `create` builds the Docker image (if not cached) and starts a persistent container (`docker run -d --name ... sleep infinity`). `run`/`shell` use `docker exec`. Container persists until `destroy`. This mirrors the VM lifecycle: create once, use many times, destroy when done.

### Q10: Where should `--type` default come from, and how should it be persisted?

The idea says `--type` defaults to `vm`. But once an environment is created, subsequent commands (`run`, `shell`, `status`, `destroy`) need to know the type without the user re-specifying it every time.

Currently, metadata is stored at `~/.isolarium/{vm-name}/repo.json`. From Q4, the identity is (name, type) — so metadata could live at `~/.isolarium/{name}/{type}/`.

**Should the type be stored in metadata so subsequent commands auto-detect it, or must the user always pass `--type`?**

- A. **Store in metadata, auto-detect** — `create` records the type in metadata. Subsequent commands look up the type by name. If both a VM and container exist with the same name, the user must specify `--type` to disambiguate; otherwise error.
- B. **Always require `--type`** — User must pass `--type container` on every command. Simple but repetitive.
- C. **Store in metadata, default to most recent** — If ambiguous, default to whichever was created or used most recently.

**Answer:** A — Store type in metadata, auto-detect on subsequent commands. `create` records the type at `~/.isolarium/{name}/{type}/metadata.json`. Commands look up by name; if both VM and container exist with the same name, user must pass `--type` to disambiguate, otherwise error.

### Q11: How should `--work-directory` interact with the container?

The idea says `--work-directory` is the host directory mounted as `~/repo` inside the container. A few details:

**Should `--work-directory` be required or optional on `create --type container`?**

- A. **Required** — Container mode always mounts a host directory. No clone, no isolation of the filesystem. This is the core value proposition (edit on host, run agent in container).
- B. **Optional, defaults to current directory** — If not specified, mount the current working directory (`. → ~/repo`). Most developers run isolarium from their project root anyway.
- C. **Optional, with fallback to clone** — If not specified, clone the repo inside the container like VM mode does. `--work-directory` overrides this with a host mount.

**Answer:** B — Optional, defaults to current working directory. If `--work-directory` is not specified, mount `cwd` as `~/repo` inside the container. The resolved path is stored in metadata so subsequent commands know the mount.

### Q12: Should `--work-directory` also be available for VM mode?

The idea mentions `--work-directory` in the context of container mode, but it could also be useful for VMs. Currently VMs clone the repo inside with no host mounts. Adding `--work-directory` to VM mode would mean mounting a host directory into the Lima VM via Lima's `mounts:` configuration.

- A. **Container only** — `--work-directory` is a container-mode feature. VM mode continues to clone the repo inside. Keep the two modes distinct in their filesystem model.
- B. **Both modes** — Allow `--work-directory` for VMs too, via Lima mounts. Gives a uniform interface but weakens the VM security model (host filesystem access).

**Answer:** A — Container only. `--work-directory` is a container-mode feature. VM mode continues to clone the repo inside with no host mounts. This preserves the distinct security models: VMs for strong isolation, containers for convenience.

### Q13: How should `status` report container environments?

Currently `status` reports: VM state (running/stopped/not created), repo info (owner/repo/branch), and GitHub App configuration. For container mode, the equivalent would be container state and work directory info.

**Should `status` list all environments (VMs and containers), or only the one matching `--name`/`--type`?**

- A. **List all** — `status` with no flags shows all environments (VMs and containers). Each entry shows name, type, state, and work directory (for containers) or repo info (for VMs).
- B. **Single environment** — `status` shows one environment, resolved by `--name` (default: "isolarium"). If both VM and container exist with that name, show both. If `--type` is specified, show only that one.
- C. **Single environment, require disambiguation** — Same as B, but if both exist, require `--type` (consistent with Q10 auto-detect rule).

**Answer:** A — List all environments. `status` with no flags shows all environments (VMs and containers). Each entry shows name, type, state, and work directory (containers) or repo info (VMs). `--name` and `--type` can filter.

### Q14: What should the Docker container naming convention be?

Lima VMs are named directly (e.g., "isolarium"). Docker containers also need a name for `docker exec`, `docker rm`, etc. The name needs to encode both the isolarium name and type to avoid collisions.

**Default assumption:** Name the container `isolarium-container-{name}`, e.g., `isolarium-container-isolarium` for the default name. The image would be named `isolarium-image-{name}` or similar.

Does this seem reasonable, or do you have a preference?

- A. **`isolarium-container-{name}`** — e.g., `isolarium-container-isolarium` for container, Lima VM stays `isolarium`. Clear but verbose.
- B. **`{name}-container`** — e.g., `isolarium-container`. Shorter.
- C. **Other convention**

**Answer:** Use whatever the `--name` argument specifies. The default for container mode is `isolarium-container` (rather than `isolarium` which remains the VM default). The Docker container name is the `--name` value directly — no prefix/suffix transformation.

### Q15: Should `destroy` for a container also remove the Docker image?

`destroy` removes the container (`docker rm -f`). But the built image remains cached, which means a subsequent `create` would be fast (no rebuild). However, the image consumes disk space.

- A. **Remove container only** — `destroy` removes the running container. Image stays cached for fast re-creation. User can manually `docker rmi` if they want to reclaim space.
- B. **Remove container and image** — `destroy` removes both. Clean slate, but next `create` requires a full rebuild.
- C. **Flag-controlled** — `destroy` removes the container by default. `destroy --clean` also removes the image.

**Answer:** A — Remove container only. Image stays cached for fast re-creation. User can manually `docker rmi` to reclaim space.

### Q16: How should the internal code be structured to support both VM and container backends?

Currently the `internal/lima/` package directly implements VM operations. To support containers, the code needs to handle two backends. From the codebase, the CLI commands in `internal/cli/` call into `internal/lima/` directly.

**Default assumption:** Introduce an interface (e.g., `Backend` or `Environment`) with methods like `Create()`, `Destroy()`, `Exec()`, `ExecInteractive()`, `Status()`, `CopyCredentials()`. The Lima package implements it for VMs, a new `internal/docker/` package implements it for containers. The CLI commands resolve the backend from the type and delegate.

Does this approach seem right, or do you have a different preference for how to structure this?

- A. **Interface + two implementations** — As described above. Clean separation, testable, standard Go pattern.
- B. **Keep it simple, if/else in CLI** — No interface. CLI commands check the type and call either `lima.*` or `docker.*` functions directly. Less abstraction, faster to implement, refactor later if needed.

**Answer:** A — Interface + two implementations. Define a `Backend` interface with methods like `Create()`, `Destroy()`, `Exec()`, `ExecInteractive()`, `Status()`, `CopyCredentials()`. `internal/lima/` implements it for VMs, new `internal/docker/` package implements it for containers. CLI resolves backend from type and delegates. This is the standard Go pattern — clean separation and testable.

### Q17: What Docker prerequisites should isolarium assume or verify?

For VM mode, isolarium manages Lima entirely (installs, creates, starts). For container mode, Docker must be installed on the host.

**Default assumption:** Require Docker (or Docker Desktop on macOS) to be pre-installed. `create --type container` verifies Docker is available (`docker info`) and errors with a helpful message if not.

On macOS, Docker Desktop is the standard way to run Docker. On Linux, Docker Engine is typically installed via the package manager.

Does this seem right, or should isolarium also handle Docker installation?

- A. **Require pre-installed Docker** — Verify with `docker info`, error if missing. Document the prerequisite.
- B. **Offer to install** — If Docker is not found, prompt the user or provide an install command.

**Answer:** A — Require pre-installed Docker. Verify with `docker info` on `create --type container`, error with helpful message if not available. Document the prerequisite.

### Q18: Should workflow tools (i2code, plugins) be included in the container image?

In VM mode, `create` clones the `humansintheloop-dev-workflow-and-tools` repo, installs custom plugins, and installs the `i2code` CLI (via `uv tool install`). These are post-creation steps that happen inside the VM.

For container mode, these could be:
- Baked into the Dockerfile (installed at image build time)
- Installed at container start time (slower `create` but same behavior as VM)
- Omitted from container mode initially

Since the Dockerfile is embedded in the binary, baking them in means the image includes everything needed.

- A. **Bake into Dockerfile** — Install workflow tools, plugins, and i2code during image build. Container is ready to use immediately after `create`.
- B. **Post-creation install** — `create` builds image then runs install steps inside the running container (like VM mode). Slower but keeps the Dockerfile simpler.
- C. **Omit initially** — Skip workflow tools for container mode. Focus on core agent functionality first. Add later if needed.

**Answer:** A — Bake into Dockerfile. Install workflow tools, plugins, and i2code during image build so the container is ready immediately after `create`.

