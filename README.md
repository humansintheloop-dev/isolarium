# Isolarium

Secure execution environment for autonomous coding agents.

Isolarium protects your workstation when running AI coding agents like Claude Code. It wraps agent sessions in an isolated environment with repo-scoped credentials so that a compromised agent cannot access your host filesystem, personal credentials, or other repositories.

## Features

- Three isolation backends with different security/speed tradeoffs:
  - VM ([Lima](https://lima-vm.io/)) — strongest isolation, separate kernel, no host mounts
  - Container (Docker) — fast provisioning, cross-platform, shared host directory
  - [Nono](https://nono.sh/) sandbox — lightweight process-level sandboxing, no provisioning overhead
- Repo-scoped credentials — GitHub App installation tokens scoped to a single repository, minted fresh per command
- Separate agent identity — agent actions use a GitHub App identity, not your personal account
- Claude Code authentication token management — copy existing token
- Disposable environments — destroy and recreate to recover from any compromise

## Prerequisites

| Tool | Install | Required for |
|------|---------|-------------|
| Go 1.22+ | [go.dev](https://go.dev/dl/) | Building from source |
| Lima | `brew install lima` | VM mode |
| Docker | `brew install docker` | Container mode |
| nono | [nono](https://nono.sh/) | Nono sandbox mode |
| GitHub App | [Creating a GitHub App](https://docs.github.com/en/apps/creating-github-apps) | Credential scoping |

## Install

```bash
git clone https://github.com/humansintheloop-dev/isolarium.git
cd isolarium
make build
```

The binary is written to `bin/isolarium`.

## Setup

Create a GitHub App for repo-scoped agent credentials and configure it in `.env.local`:

```bash
GITHUB_APP_ID=123456
GITHUB_APP_PRIVATE_KEY_PATH=/path/to/private-key.pem
```

## Quickstart

### VM mode (strongest isolation)

```bash
cd your-repo

# Create an isolated VM with the repo cloned inside
isolarium create

# Run Claude Code interactively inside the VM
isolarium run -i -- claude

# When done, tear it down
isolarium destroy
```

### Container mode (faster, cross-platform)

```bash
cd your-repo

# Create a container with the current directory mounted
isolarium create --type container

# Run Claude Code inside the container
isolarium run --type container -i -- claude

# Clean up
isolarium destroy --type container
```

### Nono sandbox mode (lightweight)

```bash
cd your-repo

# No create step needed — runs directly on host with sandboxing
isolarium run --type nono -i -- claude
```

## Commands

| Command | Description |
|---------|-------------|
| `isolarium create` | Create an isolated environment for the current repository |
| `isolarium run -- cmd` | Execute a command inside the environment |
| `isolarium shell` | Open an interactive shell for debugging |
| `isolarium status` | Show status of all environments |
| `isolarium destroy` | Delete the environment and all its contents |
| `isolarium clone-repo` | Retry repository cloning after a failed create |
| `isolarium install-tools` | Retry tool installation after a failed create |

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--type` | `vm` | Environment type: `vm`, `container`, or `nono` |
| `--name` | `isolarium` | Environment name |
| `--env-file` | `.env.local` | Path to environment file |

## `run` flags

| Flag | Default | Description |
|------|---------|-------------|
| `-i, --interactive` | `false` | Attach TTY for interactive commands |
| `--copy-session` | `true` | Copy Claude credentials from host |
| `--fresh-login` | `false` | Authenticate via device code flow instead |
| `--read` | | Grant nono sandbox read-only access to additional paths |

## License

See [LICENSE](LICENSE).
