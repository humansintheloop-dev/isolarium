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

### EC2 isolation (AWS)

The EC2 backend moves the isolated environment off the laptop entirely, so an
agent session outlives the machine that started it.
`isolarium create --type ec2` provisions a `t3.large` Ubuntu 24.04 instance
through Terraform, installs its toolchain from an embedded
[cloud-init document](internal/ec2/cloud-init.yaml), and clones the repository
inside it at `/home/ubuntu/repo`.
As with the VM backend, the instance has no host filesystem mounts — the repo is
a fully independent copy.
Commands run over SSH as the `ubuntu` user, rooted at `/home/ubuntu/repo`.

`create --type ec2` places the repository by performing the following steps:

1. Apply the Terraform configuration, reusing the shared VPC, subnet, internet
   gateway, route table, security group, and key pair when they already exist.
2. Wait for the instance to answer on SSH (up to 5 minutes), then for
   `cloud-init status --wait` to report first-boot provisioning done (up to 15
   minutes).
3. Mint a short-lived GitHub App installation token.
4. Read the git remote URL and current branch from the host working tree and
   construct an authenticated clone URL
   (`https://x-access-token:<token>@github.com/owner/repo`).
5. Run `git clone --branch <branch>` inside the instance over SSH, so no host
   credentials are exposed. The token is an argument to that one command and is
   never written to the instance's disk — `create` rewrites the clone's `origin`
   to the token-free URL, which is the one place git would otherwise persist it.
6. Configure `user.email` and `user.name` inside `/home/ubuntu/repo`, suffixing
   the name with ` - i2code` so commits authored in the environment stay
   distinguishable from commits authored on the host.
7. Copy project config files (`.claude/settings.local.json`, `CLAUDE.md`) from
   the host into the instance's `/home/ubuntu/repo`.

The instance comes pre-installed with Git, GitHub CLI, Node.js, tmux, uv, Claude
Code, and rootless Docker.

#### Agent sessions survive a closed laptop

Interactive work — `isolarium run -i` and `isolarium shell` — runs inside a tmux
session named `isolarium` on the instance, wrapped as
`tmux new-session -A -s isolarium -- <cmd>` over `ssh -t`.
Because the process belongs to the instance's tmux server rather than to the SSH
connection, closing the laptop, losing the network, or killing the SSH client
leaves the agent running.
The next `run -i` or `shell` reattaches to that same session and finds the work
where it left off; isolarium prints
`attaching to existing session 'isolarium'` to stderr when it does, because tmux
discards the command it was handed once the session already exists.

Non-interactive `isolarium run` deliberately stays outside tmux, so the exit code
and output you see are the remote command's own.

If you also run tmux on your own machine, the instance's session is nested inside
your local one, which swallows the prefix key. Press `Ctrl-b Ctrl-b` to send the
prefix through to the instance's tmux — `Ctrl-b Ctrl-b d` detaches from the
instance's session rather than your local one.

#### Claude credentials on the instance

`isolarium run --type ec2` copies your host's Claude Code credentials to
`~/.claude/.credentials.json` on the instance, as the `vm`, `container`, and
`nono` backends do.

**Security note.** That blob contains `refreshToken` and
`refreshTokenExpiresAt` — a long-lived credential to your Claude subscription,
not merely a short-lived access token. What is new with EC2 is where it lands: a
host reachable from the public internet that may stay up for weeks. Four things
limit the exposure:

- The file is written mode `0600`, so only `ubuntu` can read it.
- The instance's root volume is encrypted.
- That volume carries `delete_on_termination`, so `destroy` takes the blob with
  it.
- SSH ingress is a single `/32` rule pinned to your host's current public
  address. Isolarium never writes `0.0.0.0/0`.

None of that helps once the file is off the instance: anyone holding it has your
Claude subscription until you revoke it. Destroy environments you are finished
with.

Unlike the other backends, the EC2 copy is **conditional**. Isolarium reads the
instance's own `~/.claude/.credentials.json` first and leaves it alone unless the
host's blob expires strictly later, which protects a long-running tmux session
that refreshed its own token mid-flight from having it revoked out from under it
— while still repairing an instance whose refresh lineage the host has rotated
past. Both `expiresAt` values compared are issued by the auth server and travel
inside the blob, so no clock skew between laptop and instance enters into it.

Isolarium does not refresh the token on the instance for you. It assumes Claude
Code there renews its own access token and rewrites that file, as it does on any
other machine; that assumption is not something isolarium verifies. If a session
on the instance does lose authentication, run `isolarium run --type ec2
--copy-session` again to carry a fresh blob over from your host.

See [EC2 mode configuration](#ec2-mode-configuration) for the required
credentials and IAM permissions, cold-start latency, running cost, and teardown.

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
  half becomes the shared `aws_key_pair` every instance references. That key pair
  carries a generated name, so replacing the local keypair rotates it safely —
  the new key pair is created before any instance that has to be launched with
  it. Instances already running under the superseded key are replaced, since
  their `authorized_keys` can no longer be reached.

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

Each environment is one `t3.large` instance described by a generated
`~/.isolarium/ec2/terraform/instance-<name>.tf`, with a 50 GiB encrypted `gp3`
root volume that is deleted when the instance is terminated. Where the instance
can be reached is recorded at `~/.isolarium/<name>/ec2/metadata.json`. If
`instance-<name>.tf` already exists, `create` refuses rather than overwriting it
— run `isolarium destroy --type ec2 --name <name>` first.

`isolarium destroy --type ec2 --name <name>` removes `instance-<name>.tf`,
re-applies so Terraform terminates the instance it no longer has configuration
for, evicts the host's entry from `~/.isolarium/ec2/known_hosts`, and deletes
`~/.isolarium/<name>/ec2/`. Two consequences worth knowing:

- **An interrupted `destroy` is safe to re-run.** Removing the file and
  re-applying is self-correcting: an instance left in state with no
  configuration is always planned for destruction, so the next `destroy`
  converges. Once the environment is gone, `destroy` prints
  `no EC2 environment to destroy` and exits 0.
- **A `destroy` killed mid-apply can leave the state lock held.** Terraform
  reports the lock ID; clear it with
  `terraform -chdir=~/.isolarium/ec2/terraform force-unlock <id>` and re-run
  `destroy`. Only do this once you are certain no other isolarium invocation is
  still running.
- **An apply killed between an AWS call and the state write orphans that
  resource.** Terraform will try to create it again on the next apply and AWS
  will refuse — an orphaned subnet, for instance, fails the next `create` with
  `InvalidSubnet.Conflict`. Either `terraform import` the orphan into state, or
  delete it in the console once you have confirmed nothing is using it, and
  re-run.

Teardown also tolerates a failure to detect your public IP: it warns and falls
back to the CIDR persisted in `isolarium.auto.tfvars`, so being off the network
you created from never strands a billing instance. With no persisted value it
fails rather than widening ingress.

Project setup steps run at the end of `create --type ec2`, declared under an
`ec2` key in the repository's `pid.yaml`:

```yaml
isolarium:
  ec2:
    create:
      creation_scripts:
        - path: scripts/isolation/install-go.sh
        - path: scripts/isolation/install-codescene.sh
          env:
            - CS_ACCESS_TOKEN
      post_creation_scripts:
        host_scripts: []
        env_scripts: []
    run:
      env:
        - CS_ACCESS_TOKEN
```

`creation_scripts` and `post_creation_scripts.env_scripts` run inside the
instance from `/home/ubuntu/repo`, so their paths are relative to the repository
root and have to be committed to the branch being cloned.
`post_creation_scripts.host_scripts` run on the host, relative to the directory
`create` was run from. Every script sees `ISOLARIUM_NAME` and
`ISOLARIUM_TYPE=ec2`, plus the host variables it names under `env` — an unset
one fails the `create` rather than running the script without it. A path that
escapes the project root is rejected when `pid.yaml` is read, before the AWS
account is touched.

Two things worth knowing before your first `create --type ec2`:

- **Cold start takes minutes, not seconds.** The instance is built from a stock
  Ubuntu image at apply time rather than from a pre-baked AMI, and `create` does
  not return until cloud-init has finished installing the toolchain. Measured
  runs took between 1m36s and 2m18s; `create` gives up after 15 minutes.
- **Instances bill until you destroy them.** Isolarium has no idle auto-stop and
  no cost reporting. A forgotten `t3.large` with a 50 GiB `gp3` volume costs
  roughly $64/month. Run `isolarium destroy --type ec2 --name <name>` when you
  are done with an environment.

### Testing EC2 mode against a real account

The EC2 lifecycle tests are behind the `ec2` build tag, so `go test ./...` and CI
can never launch a billable instance. Nothing in `.github/workflows/ci.yml` needs
AWS credentials.

```bash
ISOLARIUM_EC2_INTEGRATION=1 ./test-scripts/test-ec2.sh
```

The script refuses to run without `ISOLARIUM_EC2_INTEGRATION=1`, and fails when
`go test` selected no test rather than reporting a green run over nothing. The
tests themselves fail — they never skip — when `AWS_REGION`,
`AWS_ACCESS_KEY_ID`, or `AWS_SECRET_ACCESS_KEY` is missing. The test that proves
`claude` authenticates on the instance fails the same way when your host has no
Claude credentials to copy: it reads them from the Keychain, falls back to the
file named by `CLAUDE_CREDENTIALS_PATH`, and fails when neither is there. Every
test registers a cleanup that destroys its instance even after a failed
assertion, so a red run does not leave one billing.

`./test-scripts/test-end-to-end.sh --with-ec2` adds the same script to the
end-to-end suite; without the flag the suite stays AWS-free. `make test-ec2` runs
the tagged tests directly.

Every test builds its own billable instance, so re-running one after a failure
should not have to rebuild all of them. An optional argument narrows the run:

```bash
ISOLARIUM_EC2_INTEGRATION=1 ./test-scripts/test-ec2.sh TestEC2Instance_ClaudeAuthenticates
```

A narrowed run that matches nothing still fails, because the script rejects a
`go test` that reported `no tests to run`.

One failure worth recognising: `create "<name>": instance did not finish
cloud-init within 15m0s` while the log fills with `status: done`. That is an
instance whose cloud-init finished degraded — `cloud-init status --wait` prints
`done` but exits non-zero — which the readiness loop cannot distinguish from one
still provisioning. It is an unlucky instance rather than a broken test; re-run
that single test by name.

The run reports three timings you should expect to see in the output: `TIMING:
create` (the `terraform apply` wall clock, which includes waiting for cloud-init
to finish), `TIMING: cold start from create to first SSH login`, and `TIMING:
cloud-init reported done`. It also reports `SIZE: rendered user_data`.

Measured on 2026-08-20 in `us-west-1` against a real account, on a run where the
shared VPC, subnet, gateway, route table, security group, and key pair already
existed and only the instances had to be built:

| Measurement | Value |
| --- | --- |
| `go test -tags=ec2 ./internal/ec2/...` | 1046s for all seven instances |
| Lifecycle test | 155s, of which `TIMING: create` was 1m56s |
| Repository test | 135s, of which `TIMING: create` was 1m45s |
| Persisted-token test | 141s, of which `TIMING: create` was 1m52s |
| Claude-authenticates test | 151s, of which `TIMING: create` was 1m50s |
| Survives-disconnect test | 163s, of which `TIMING: create` was 1m55s |
| New-session test | 153s, of which `TIMING: create` was 1m50s |
| Toolchain test | 147s, of which `TIMING: create` was 1m45s |
| `TIMING: cloud-init reported done` | 1m47s after create started |
| `SIZE: rendered user_data` | 3418 bytes of the 16384-byte limit |

Each test builds its own instance, so the wall clock is roughly the instance
count times the cold start. Earlier runs measured `TIMING: create` as high as
2m18s.

EC2 caps `user_data` at 16 KB, which makes that limit a live constraint on
`internal/ec2/cloud-init.yaml` rather than a theoretical one. The document
currently spends about a fifth of the budget, so the toolchain has room to grow —
but a substantial addition should be measured against the reported size rather
than assumed to fit.

The run exited 0: `Exec` of `echo hello` returned `hello` with exit code 0,
`Exec` of `exit 42` returned 42, `DescribeInstances` reported the instance
`terminated` after `destroy`, and on a freshly created instance `cloud-init
status --wait` reported `status: done` while `git --version`, `gh --version`,
`node --version`, `tmux -V`, `uv --version`, `claude --version`, and a rootless
`docker info` each exited 0 with
`kernel.apparmor_restrict_unprivileged_userns = 0`. On the instances created
from this repository's own checkout, `/home/ubuntu/repo` was on the branch
`create` ran from, `git config user.name` there carried the ` - i2code` suffix,
no tracked file was modified, `.claude/settings.local.json` and `CLAUDE.md` had
travelled from the host, and a recursive search of `/home/ubuntu` found the
installation token in no file at all. On the disconnect instance, a
once-per-second writer started through `run -i` kept the same PID and kept
growing its log after its local `ssh` process was `SIGKILL`ed, and
`tmux list-sessions` reported exactly one session named `isolarium` both before
the disconnect and after the reattach. Expect the first run in a fresh account to
take longer, because that apply also builds the shared network.

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
| `--new-session` | `false` | Start an additional tmux session (`isolarium-<n>`) on the instance instead of joining the running one; `ec2` only, and never kills a session |
| `--work-directory` | cwd | Work directory to mount (container mode, requires `--create`) |

## License

See [LICENSE](LICENSE).
