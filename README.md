<p align="center">
  <img src="isolarium_logo.png" alt="Isolarium" width="400">
</p>

# Isolarium

Secure execution environment for autonomous coding agents.

Isolarium protects your workstation when running AI coding agents like Claude Code. It wraps agent sessions in an isolated environment with repo-scoped credentials so that a compromised agent cannot access your host filesystem, personal credentials, or other repositories.

## Features

- Four isolation backends with different security/speed tradeoffs:
  - VM ([Lima](https://lima-vm.io/)) — strongest isolation, separate kernel, no host mounts (macOS only)
  - Container (Docker) — fast provisioning, cross-platform, shared host directory
  - [Nono](https://nono.sh/) sandbox — lightweight process-level sandboxing, no provisioning overhead
  - EC2 isolation (AWS) — remote Linux instance, isolation off your machine entirely
- Repo-scoped credentials — GitHub App installation tokens scoped to a single repository, minted fresh per command
- Separate agent identity — agent actions use a GitHub App identity, not your personal account
- Claude Code authentication token management — copy existing token
- Disposable environments — destroy and recreate to recover from any compromise

## How it works

Isolarium commands operate on the current **working tree** — run `isolarium create` and `isolarium run` from the root of the repository you want to isolate.

- `create` — provisions an isolated environment and mounts (or clones) the current working tree into it
- `run` — executes a command inside that environment with repo-scoped credentials
- `destroy` — tears it down

### VM isolation (Lima, macOS only)

The VM backend provides the strongest isolation.
`isolarium create` provisions an Ubuntu 24.04 virtual machine via Lima (see [`template.yaml`](internal/lima/template.yaml)) and clones the repository inside it at `~/repo`.
The VM has no host filesystem mounts — the repo is a fully independent copy.
Commands run over SSH through `limactl shell`.

Isolarium clones the repository by performing the following steps:

1. Mint a short-lived GitHub App installation token.
2. Read the git remote URL and current branch from the host working tree and construct an authenticated clone URL (`https://x-access-token:<token>@github.com/owner/repo`), converting SSH URLs to HTTPS if needed.
4. Run `git clone` inside the VM via `limactl shell`, so no host credentials are exposed.
5. Copy project config files (`.claude/settings.local.json`, `CLAUDE.md`) from the host into the VM's `~/repo`.

The VM comes pre-installed with Git, Node.js, GitHub CLI, Docker (rootless), Java 17, and Gradle.
Custom setup steps can be added via `isolation_scripts` in `pid.yaml`.

### Container isolation (Docker)

The container backend bind-mounts the current working tree into a Docker container at `/home/isolarium/repo`.
`isolarium create` builds an image from an embedded [Dockerfile](internal/docker/Dockerfile) (Ubuntu 24.04 base, same toolchain as VM) and starts a long-running container.

The container runs as a non-root user whose UID matches the host, drops all Linux capabilities (`--cap-drop=ALL`), and prevents privilege escalation (`--security-opt=no-new-privileges`).
If the working tree is a git worktree, isolarium detects this and bind-mounts the main repository as well.

### Nono sandbox isolation

The nono backend requires no `create` step.
It wraps commands with the [nono](https://nono.sh/) capability-based sandbox using an embedded [profile](internal/nono/isolarium-nono-profile.json), which restricts filesystem access at the syscall level while running directly on the host.

The sandbox:

- Grants read-write access to the current working tree and `~/.claude`
- Grants read-only access to caches and configuration files (git, gh, SDKMAN)
- Grants read-only access to Claude plugin marketplaces installed outside `~/.claude` (read from `~/.claude/plugins/known_marketplaces.json`)
- Blocks everything else
- Allows additional read-only paths at runtime with `--read`
- Does not restrict networking

### Git and GitHub credentials

Isolarium keeps agent credentials separate from your personal GitHub identity.
A GitHub App provides repo-scoped tokens so the agent can push code and open PRs without access to your other repositories.

**Token minting** (VM and nono): isolarium reads the app's private key from `GITHUB_APP_PRIVATE_KEY_PATH`, signs a JWT, and calls the GitHub API to mint a short-lived installation token scoped to the current repository.
The token is injected as `GH_TOKEN` and, for git operations, configured via environment variables so no credentials are written to disk.

**Token extraction** (container): isolarium reads the token from your existing `gh` CLI session (`gh auth token`) and injects it as `GH_TOKEN`.
The container's git is pre-configured to delegate authentication to `gh auth git-credential`.

The `--no-gh-token` flag disables all token injection for commands that should run without GitHub access.

## Prerequisites

| Tool | Install | Required for |
|------|---------|-------------|
| Go 1.24+ | [go.dev](https://go.dev/dl/) | Building from source |
| Lima | `brew install lima` | VM mode (macOS only) |
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

### EC2 mode configuration

EC2 mode reads AWS credentials from the same `.env.local` file. There is no
isolarium-specific AWS profile knob — on SSO, export the env-var form with
`aws configure export-credentials`.

| Variable | Required | Purpose |
|----------|----------|---------|
| `AWS_ACCESS_KEY_ID` | yes | Consumed natively by Terraform and the AWS SDK |
| `AWS_SECRET_ACCESS_KEY` | yes | Consumed natively by Terraform and the AWS SDK |
| `AWS_SESSION_TOKEN` | no | For temporary or SSO credentials |
| `AWS_REGION` | yes | No implicit default; `create --type ec2` fails fast when unset |

On the first `create --type ec2`, isolarium bootstraps a Terraform remote-state
bucket named `isolarium-tfstate-<account-id>-<region>`, with versioning enabled,
`AES256` encryption, and all four public-access-block flags set. The bootstrap is
idempotent — an existing bucket you already own is reused.

The same first `create` also provisions host-side state under `~/.isolarium/ec2/`:

- The Terraform working directory at `~/.isolarium/ec2/terraform/` is extracted
  from the binary once. Later runs leave it alone, so edits you make there
  survive — delete a file to have isolarium restore its shipped version.
- An Ed25519 keypair is generated at `~/.isolarium/ec2/id_ed25519` (mode `0600`)
  and `id_ed25519.pub` (mode `0644`). Both are reused once present; the public
  half becomes the shared `aws_key_pair` every instance references.

SSH ingress is restricted to your host's current public address, detected via
`https://checkip.amazonaws.com` and applied as a single `/32`. There is one
shared ingress rule for all instances, so **switching networks and then running
any `create` or `destroy` re-points ingress and restores access to every
instance**. Isolarium never writes `0.0.0.0/0`: on `create`, failed detection is
fatal. Each successful detection is persisted to
`~/.isolarium/ec2/terraform/isolarium.auto.tfvars`.

These credentials must carry the following IAM permissions:

- `sts:GetCallerIdentity`
- On the state bucket: `s3:CreateBucket`, `s3:PutBucketVersioning`,
  `s3:PutEncryptionConfiguration`, `s3:PutBucketPublicAccessBlock`,
  `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`, `s3:ListBucket`
- `ssm:GetParameter` for the AMI lookup
- The EC2 and VPC permissions to create, describe, tag, and delete instances,
  key pairs, security groups, VPCs, subnets, internet gateways, and route tables

No `dynamodb:*` permission is required — state locking uses S3 conditional writes.

## Quickstart

## With Idea to Code

The goal of Isolarium is to support the [Idea to Code workflow](https://github.com/humansintheloop-dev/humansintheloop-dev-workflow-and-tools).

Specifically, the `i2code implement` command:

```bash
$ i2code implement --isolation-type nono/container/vm my-idea
```

This will implement the idea in an isolated environment of the specified type.

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
| `--create` | `false` | Create the environment if it does not exist |
| `--work-directory` | cwd | Work directory to mount (container mode, requires `--create`) |

## License

See [LICENSE](LICENSE).
