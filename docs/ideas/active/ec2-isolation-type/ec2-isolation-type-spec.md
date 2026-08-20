# EC2 Isolation Type — Platform Capability Specification

**Idea type: C — Platform / infrastructure capability.**

Determined from the discussion file (Q16, reaffirmed throughout). The work adds a fourth `Backend` implementation alongside `LimaBackend`, `DockerBackend`, and `NonoBackend` against the unchanged interface in `internal/backend/backend.go:14`. User-facing command semantics do not change: `isolarium create --type ec2`, `run --type ec2`, `destroy --type ec2` behave as their `vm` counterparts. The substance of the work is AWS infrastructure — Terraform configuration, remote state, networking, credential handling, SSH transport — not new product workflow. One new command (`isolarium ec2 wipe`) exists purely for infrastructure lifecycle, which is itself platform-shaped.

---

## 1. Purpose and Context

### 1.1 Purpose

Provide an `ec2` isolation backend that runs an isolated coding-agent environment on an Amazon EC2 instance instead of a local Lima VM, Docker container, or nono sandbox.

### 1.2 Goals

1. **Long-running agent sessions survive laptop sleep and SSH disconnect.** The agent process runs inside a `tmux` session on the instance. Closing the laptop drops the SSH connection; the process keeps running. The next `isolarium run -i` re-attaches to it.
2. **Access compute larger than the laptop.** A fixed cloud instance size larger than a typical developer machine satisfies this for v1.

Cross-platform reach and team-shared infrastructure are not v1 drivers. Nothing in this specification blocks them later.

### 1.3 Context in the existing system

The `Backend` interface (`internal/backend/backend.go:14`) defines seven operations: `Create`, `Destroy`, `Exec`, `ExecInteractive`, `OpenShell`, `GetState`, `CopyCredentials`. An **environment** is the `(name, type)` pair; identity lives on disk at `~/.isolarium/<name>/<type>/`. For this backend, one environment is exactly one EC2 instance.

Every existing backend shells out to a CLI (`limactl`, `docker`, `nono`). This backend shells out to `terraform` and `ssh`, keeping that pattern intact.

---

## 2. Consumers

| Consumer | How it consumes the capability |
|---|---|
| **Isolarium CLI end user (developer)** | Runs `isolarium create/run/shell/destroy/status --type ec2` from a project checkout. Primary consumer. |
| **`internal/cli` command layer** | Resolves `"ec2"` through `backend.ResolveBackend` (`internal/backend/resolve.go:16`) and calls the `Backend` interface. Requires no knowledge of AWS. |
| **`internal/status` reporting** | Calls `GetState("<name>")` through the `StateProvider` func and reads `~/.isolarium/<name>/ec2/metadata.json` for descriptive fields. |
| **`pid.yaml` project configuration** | Projects declare `isolarium.ec2.create.creation_scripts`, `isolarium.ec2.create.post_creation_scripts`, and `isolarium.ec2.run.env`, mirroring the existing `vm` section. |
| **CI / integration test suite** | `test-scripts/test-ec2-integration.sh` exercises the capability against real AWS, gated on explicit opt-in. |
| **The user's AWS account** | Receives all created infrastructure. Must be an account the user is authorized to create VPC, EC2, and S3 resources in. |

---

## 3. Capabilities and Behaviors

### 3.1 Infrastructure model

A single Terraform working directory at `~/.isolarium/ec2/terraform/` describes **all** AWS infrastructure. One shared state file holds everything.

```
~/.isolarium/ec2/
  terraform/
    provider.tf              # aws provider, default_tags, required_version >= 1.10
    backend.tf               # backend "s3" with use_lockfile = true
    network.tf               # VPC, public subnet, IGW, route table, association
    security.tf              # base SG + the single shared SSH ingress rule + egress
    keypair.tf               # aws_key_pair from var.public_key
    variables.tf             # ingress_cidr, public_key, region
    isolarium.auto.tfvars    # last successfully detected ingress_cidr (see 3.4)
    instance-<name>.tf       # generated per environment; removed on destroy
  id_ed25519                 # 0600
  id_ed25519.pub             # 0644
  known_hosts                # 0600, dedicated
```

Stable scaffolding files are embedded in the binary with `go:embed` and extracted **once** into that directory, mirroring how `internal/lima/template.yaml` is embedded today. They are not re-extracted on subsequent runs, so a user may inspect them.

**No IAM instance profile is created.** This is a deliberate deviation from the Q19 sketch, which listed `iam.tf`. The instance makes no AWS API calls: the AMI is resolved by a Terraform `data "aws_ssm_parameter"` source using the *user's* credentials, cloud-init needs no AWS access, and SSM Session Manager is out of scope. An instance profile with no attached policies would be dead weight. If a future capability needs one, adding `iam.tf` is additive.

### 3.2 State backend bootstrap (outside Terraform)

The S3 state bucket is created directly via the AWS SDK, because Terraform cannot store its state in a bucket that does not yet exist.

- **Bucket name:** `isolarium-tfstate-<account-id>-<region>`, where `<account-id>` comes from STS `GetCallerIdentity`.
- **Operations, in order, idempotent:** `CreateBucket` → `PutBucketVersioning` (Enabled) → `PutBucketEncryption` (SSE-S3, `AES256`) → `PutPublicAccessBlock` (all four flags true).
- An existing bucket owned by the caller is accepted and reused; `BucketAlreadyOwnedByYou` is not an error.
- The bucket is **never** managed or destroyed by Terraform.

**Locking uses Terraform's native S3 lockfile** (`use_lockfile = true`), introduced in Terraform 1.10. There is no DynamoDB table. This supersedes the DynamoDB design in Q19.

### 3.3 Terraform invocation

- Isolarium shells out to the `terraform` binary.
- Before any Terraform operation, isolarium runs `terraform version -json`, parses `.terraform_version`, and fails with an actionable message if it is below `1.10.0`.
- `terraform init` is run when `~/.isolarium/ec2/terraform/.terraform/` is absent, and is idempotent.
- Every `apply` and `destroy` passes `-lock-timeout=120s` and `-input=false`, so a concurrent invocation waits rather than failing immediately.
- Terraform inherits AWS credentials from the process environment; no credential wiring code exists.

### 3.4 SSH ingress

Isolarium detects the host's public IP **host-side, in Go** — an HTTPS GET to `https://checkip.amazonaws.com`, body trimmed, `/32` appended. Terraform makes no external HTTP calls of its own.

Detection runs before **every** `terraform apply` and `terraform destroy`, regardless of `--name`, and the result is passed as `-var="ingress_cidr=<ip>/32"`.

Because all instances share one CIDR value, there is **one** ingress rule, in the stable `security.tf`, referencing `var.ingress_cidr`. Generated per-instance files contain no ingress rule. A consequence, and a feature: any create or destroy re-points ingress for every instance, so switching networks and then running any operation restores access to all of them.

**Detection-failure policy differs by operation, so that teardown is never blocked:**

| Operation | Detection fails |
|---|---|
| `create` | **Fatal.** Clear error. Never falls through to `0.0.0.0/0`. |
| `destroy`, `ec2 wipe` | **Warn and continue** using the last-known value persisted in `isolarium.auto.tfvars`. If no persisted value exists, fatal. |

On every successful detection, the value is written to `~/.isolarium/ec2/terraform/isolarium.auto.tfvars` as `ingress_cidr = "<ip>/32"`. Terraform loads `*.auto.tfvars` automatically; the explicit `-var` on the command line takes precedence when present.

### 3.5 SSH transport

Isolarium uses the **system `ssh` binary**. `crypto/ssh` was rejected: attaching to tmux requires PTY allocation, raw terminal mode, and SIGWINCH propagation, all of which `ssh -t` provides.

A single helper builds the option set so `Exec`, `ExecInteractive`, `OpenShell`, and `CopyCredentials` cannot drift, mirroring how `internal/lima/ssh.go` centralizes `BuildShellCommand`:

```
ssh
  -i ~/.isolarium/ec2/id_ed25519
  -o UserKnownHostsFile=<isolarium ec2 dir>/known_hosts
  -o StrictHostKeyChecking=accept-new
  -o IdentitiesOnly=yes
  -o ConnectTimeout=10
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=4
  [-t]                       # ExecInteractive and OpenShell only
  ubuntu@<public_dns>
  -- <remote command>
```

`ServerAliveInterval=15` with `ServerAliveCountMax=4` tears down a dead connection after ~60s rather than hanging. The remote process is unaffected because it runs inside tmux.

**Host-key policy:** a dedicated `known_hosts` under `~/.isolarium/ec2/` with `accept-new`. The user's `~/.ssh/known_hosts` is never touched. First connection to a new host is trusted; a *changed* key on a known host fails loudly. `destroy` evicts the entry with `ssh-keygen -R <public_dns> -f <known_hosts>`, so AWS recycling that DNS name later does not produce a mismatch that looks like an attack.

**Keypair:** on first `create`, isolarium generates an Ed25519 keypair host-side, writes the private half to `~/.isolarium/ec2/id_ed25519` (`0600`) and the public half to `id_ed25519.pub` (`0644`). The public key is passed as `-var="public_key=<contents>"`; Terraform creates the `aws_key_pair` that every instance references. If the private key already exists, it is reused.

### 3.6 Session persistence

**One tmux session per instance**, fixed name `isolarium`.

- `ExecInteractive` and `OpenShell` wrap the command: `tmux new-session -A -s isolarium -- <cmd>`, run over `ssh -t`.
- Non-interactive `Exec` does **not** use tmux — exit codes and stdout are unreliable through a tmux attach.
- A session whose command exits disappears on its own, so a finished session never blocks the next run.
- `tmux new-session -A` silently discards the passed command when a session already exists. To remove that surprise, isolarium runs `tmux has-session -t isolarium` first and, if present, prints to stderr:
  `attaching to existing session 'isolarium'; use --new-session to start a fresh one`
  This is a notice, not a prompt — there is no interactive fallback path to maintain.
- **`--new-session` starts an additional session and never kills anything.** The new session is named `isolarium-<n>`, where `<n>` is the lowest integer ≥ 2 not currently in use. Non-destructive behavior is chosen because destroying a running agent session is exactly the outcome this backend exists to prevent.
- `--new-session` is a flag on `run` and `shell`, and is rejected with a clear error for non-`ec2` types.

### 3.7 Image and provisioning

- **AMI:** resolved at apply time by `data "aws_ssm_parameter"` at `/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id`. No hardcoded AMI IDs.
- **Architecture: always x86_64.** One instance type, one AMI path, no `runtime.GOARCH` branch, no two-arch test matrix. This supersedes Q11's "arch follows the host."
- **Instance type:** `t3.large`, hardcoded. No override flag in v1.
- **Root volume:** 50 GiB `gp3`, `encrypted = true`, `delete_on_termination = true`.
- **Provisioning:** a new `internal/ec2/cloud-init.yaml`, embedded with `go:embed`, delivered as `user_data`. It is a **duplicate** of the Lima provisioning content, adapted — not a shared source of truth. `internal/lima/template.yaml` is not modified, so this work carries no VM regression risk.
- Adaptations from the Lima template: root-level steps become `packages:` and `runcmd:`; user-level steps run explicitly as `ubuntu` rather than via `$USER`; `tmux` is added.
- Toolchain parity with the Lima template: git, curl, wget, ca-certificates, gnupg, lsb-release, unzip, zip, uidmap, dbus-user-session, the `kernel.apparmor_restrict_unprivileged_userns=0` sysctl, Node.js LTS, GitHub CLI, rootless Docker, `loginctl enable-linger`, SDKMAN, Claude Code via npm, uv — plus `tmux`.
- **`user_data` size is validated host-side before apply.** If the rendered document exceeds 16384 bytes, `create` fails with an explicit error naming the limit. This converts a confusing AWS API rejection into a clear one.

### 3.8 Environment naming

`--name` for `--type ec2` must match `^[a-z][a-z0-9-]{0,31}$`. Invalid names are rejected at the CLI with a message stating the rule.

This regex is deliberately chosen so the name is simultaneously a valid Terraform identifier, a valid filename component, and a valid AWS tag value — **no sanitisation or name mangling is required anywhere.**

- Generated file: `instance-<name>.tf`
- Instance resource address: `aws_instance.<name>`
- Terraform outputs declared in that same file: `output "instance_id_<name>"` and `output "public_dns_<name>"`

Per-instance outputs are declared inside each generated file rather than aggregated, because Terraform does not permit appending to a shared `output` block from multiple files.

**Default `--name` for `ec2` is `isolarium-ec2`**, added as `defaultEC2Name` alongside `defaultContainerName` and `defaultNonoName` (`internal/cli/cmd_create.go:11`) and returned by `resolveDefaultName`.

### 3.9 Lookup and state

`create` writes `~/.isolarium/<name>/ec2/metadata.json` from `terraform output -json`:

```json
{
  "instance_id": "i-0123456789abcdef0",
  "public_dns": "ec2-203-0-113-7.compute-1.amazonaws.com",
  "region": "us-east-1",
  "owner": "humansintheloop-dev",
  "repo": "isolarium",
  "branch": "main",
  "created_at": "2026-08-19T14:03:21Z"
}
```

- `run` and `shell` read this file and connect directly. **They require neither AWS credentials nor the `terraform` binary.**
- **Refresh-on-failure:** the instance ID is immutable; the public DNS is not. If SSH fails to *connect* (as distinct from the remote command exiting non-zero), isolarium calls `DescribeInstances` by the cached instance ID, rewrites `metadata.json`, and retries the SSH operation exactly once. This recovers from a stop/start or an out-of-band apply without paying a lookup on every command.
- `GetState(name)` reads the metadata file for the instance ID and calls `DescribeInstances`, mapping AWS instance states to the strings the existing status layer expects (`internal/lima/lima.go:48` returns `"running"`, `"stopped"`, or `"none"`):

| AWS state | Returned |
|---|---|
| `running` | `running` |
| `stopped`, `stopping` | `stopped` |
| `pending` | `pending` |
| `shutting-down`, `terminated` | `none` |
| metadata file absent | `none` |
| AWS call fails or credentials missing | `unknown` |

`status` integration is two changes: add `"ec2"` to `knownTypes` (`internal/status/environment.go:41`) and an `ec2` case in `populateTypeSpecificFields` populating `Repository` and `Branch` from the same fields the `vm` case uses.

### 3.10 Repository placement

Mirrors the Lima flow in `internal/cli/vm_setup.go:31`:

1. Resolve repo info from the working directory (`git.GetRemoteURL`, `git.GetCurrentBranch`, `github.ParseRepoURL`).
2. Push the current branch to the remote (`git.PushBranch`).
3. Mint a short-lived GitHub App installation token host-side.
4. `git clone` **inside** the instance over SSH, token passed in the URL for that single command, so it never lands on the instance's disk.
5. Configure the git author inside the instance using `git.TransformEmailForIsolation` and the `" - i2code"` name suffix, matching `configureVMGitAuthor`.
6. Copy `.claude/settings.local.json` and `CLAUDE.md` from the host into the instance.
7. Write `metadata.json`.

Repository path inside the instance: `/home/ubuntu/repo`, matching the Lima convention of `<home>/repo`.

Uncommitted host changes are not carried into the instance — the same constraint the VM backend has.

### 3.11 Claude Code credentials

`CopyCredentials` for EC2 is **conditional**, diverging deliberately from Lima's unconditional write.

Algorithm on each `run`:

1. Read `~/.claude/.credentials.json` from the instance over SSH.
2. Parse `claudeAiOauth.expiresAt` (epoch millis).
3. Write the host blob when: the file is absent, OR unparseable, OR the host's `expiresAt` is **strictly greater** than the instance's. Otherwise leave the instance untouched.
4. When writing: `mkdir -p ~/.claude`, write the file, `chmod 600`.

`expiresAt` is issued by the auth server and travels inside the credential blob, so this compares two server-issued values — **not two machine clocks**. There is no laptop-versus-EC2 skew concern.

This protects a long-running tmux session that refreshed its own token mid-flight, while still repairing an instance whose refresh lineage the host rotated out from under it.

**Security note that must appear in the README:** the Keychain blob copied to the instance contains `refreshToken` and `refreshTokenExpiresAt` — a long-lived credential to the user's Claude subscription. This is already true of the `vm`, `container`, and `nono` backends, but EC2 places it on a public-internet-reachable host that may run for weeks. Q6 previously recorded the opposite; that record is corrected in Q31.

### 3.12 Post-creation and creation scripts

Parity with the VM backend, reusing the existing `envscript` and `hostscript` packages.

`internal/config/pidconfig.go` gains an `EC2 IsolationTypeConfig` field with tag `yaml:"ec2"`, and `validateConfig` gains the three corresponding path-validation sections (`ec2.create.creation_scripts`, `ec2.create.post_creation_scripts.host_scripts`, `ec2.create.post_creation_scripts.env_scripts`) so that path-escape validation covers them.

`loadRunEnvVarsImpl` (`internal/cli/cmd_run.go:31`) gains an `"ec2"` case returning `cfg.EC2.Run.Env`.

`hostscript.RunHostScripts` is called with isolation type `"ec2"`; `envscript.RunEnvScripts` likewise.

### 3.13 Lifecycle operations

**`create`**

1. Validate `--name`.
2. Require `AWS_REGION`; fail fast if unset.
3. Check `terraform` version ≥ 1.10.
4. Ensure the S3 state bucket exists (SDK, idempotent).
5. Extract embedded scaffolding into `~/.isolarium/ec2/terraform/` if absent.
6. Generate the SSH keypair if absent.
7. Detect the public IP; fatal on failure. Persist to `isolarium.auto.tfvars`.
8. Fail if `instance-<name>.tf` already exists, mirroring `lima.CreateVM`'s refusal to overwrite an existing VM.
9. Write `instance-<name>.tf`.
10. `terraform init` if needed, then `terraform apply -auto-approve -input=false -lock-timeout=120s -var=... `.
11. Read `terraform output -json`; write `metadata.json`.
12. Poll SSH until reachable (see 5.2 for budgets).
13. Run `cloud-init status --wait` over SSH.
14. Clone the repo, configure git author, copy project config.
15. Run `ec2.create.creation_scripts`, then post-creation host scripts and env scripts.

**`destroy`**

1. Detect the public IP; on failure warn and fall back to the persisted value.
2. Remove `instance-<name>.tf`.
3. `terraform apply` with the same flags.
4. Evict the `known_hosts` entry for the recorded public DNS.
5. Remove `~/.isolarium/<name>/ec2/`.

Remove-file-then-apply is chosen over targeted destroy because it is self-correcting: a resource present in state with no configuration is always planned for destruction, so an interrupted destroy completes on retry. The rejected alternative has a failure mode where a successful destroy followed by a failed file removal causes the **next create of any other name to resurrect the destroyed instance.**

If `instance-<name>.tf` does not exist, `destroy` prints `no EC2 environment to destroy` and exits 0, mirroring `destroyVM` (`internal/cli/cmd_destroy.go:35`).

**`isolarium ec2 wipe`**

1. Enumerate `instance-*.tf` in the Terraform directory.
2. **If any exist, refuse**, listing them and instructing the user to `isolarium destroy --type ec2 --name <each>` first. Refusing is chosen over cascading destruction because `destroy` is the per-instance verb and a wipe that silently terminates running agent sessions is the wrong default.
3. Detect the public IP with the destroy-time fallback policy.
4. `terraform destroy -auto-approve -input=false -lock-timeout=120s`.
5. Remove `~/.isolarium/ec2/terraform/`, `id_ed25519`, `id_ed25519.pub`, and `known_hosts`.
6. Print that the S3 state bucket was **intentionally retained**, naming it, and point at the README for manual removal.

The bucket survives because it costs approximately nothing, is harmless to reuse, and the bootstrap is idempotent. Deleting a versioned bucket requires enumerating and deleting every object version, which is not worth carrying in v1.

---

## 4. APIs, Contracts, and Integration Points

### 4.1 Backend interface

`EC2Backend` implements `backend.Backend` unchanged. `ResolveBackend` (`internal/backend/resolve.go:16`) gains a `case "ec2"`.

Per the pattern in `newDockerBackend` and `newNonoBackend`, `EC2Backend` takes injectable function fields so unit tests need neither AWS nor SSH — at minimum: SSH exec, SSH interactive exec, `DescribeInstances`, Terraform invocation, public-IP detection, and clock.

### 4.2 CLI surface

| Command | Change |
|---|---|
| `--type` flag | `environmentType.Set` (`internal/cli/environment_type.go:13`) accepts `"ec2"`; the error message and the two `--type` flag descriptions in `internal/cli/root.go:79,105` are updated. |
| `create` | Routes `ec2` to a `createAndSetupEC2` path analogous to the existing `vm` special case (`internal/cli/cmd_create.go:34`). `--work-directory` is rejected for `ec2`, as it is for `vm` and `nono`. |
| `destroy` | Routes `ec2` alongside the existing `vm` special case. |
| `run`, `shell` | Gain `--new-session`, valid only for `ec2`. Neither auto-creates an EC2 environment; both fail with a clear message directing the user to `create`. |
| `status` | No command change; `"ec2"` added to `knownTypes`. |
| `ec2 wipe` | New subcommand under a new `ec2` command group. |

### 4.3 Configuration contract

`.env.local` (already loaded by isolarium):

| Variable | Required | Purpose |
|---|---|---|
| `AWS_ACCESS_KEY_ID` | yes | Consumed natively by Terraform and the SDK |
| `AWS_SECRET_ACCESS_KEY` | yes | " |
| `AWS_SESSION_TOKEN` | no | For temporary/SSO credentials |
| `AWS_REGION` | yes | No implicit default; fail fast when unset |
| `GITHUB_APP_ID` | yes | Existing; for repo clone |
| `GITHUB_APP_PRIVATE_KEY_PATH` | yes | Existing |

There is no isolarium-specific AWS profile knob. Users on SSO export the env-var form (`aws configure export-credentials`).

`pid.yaml` gains an `ec2` section structurally identical to `vm`:

```yaml
isolarium:
  ec2:
    create:
      creation_scripts:
        - path: scripts/isolation/install-go.sh
      post_creation_scripts:
        host_scripts: []
        env_scripts: []
    run:
      env:
        - CS_ACCESS_TOKEN
```

### 4.4 AWS IAM permissions required of the user's credentials

`sts:GetCallerIdentity`; `s3:CreateBucket`, `s3:PutBucketVersioning`, `s3:PutEncryptionConfiguration`, `s3:PutBucketPublicAccessBlock`, `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`, `s3:ListBucket` on the state bucket; `ssm:GetParameter` for the AMI lookup; and the EC2 and VPC permissions to create, describe, tag, and delete instances, key pairs, security groups, VPCs, subnets, internet gateways, and route tables. **No `dynamodb:*` is required.**

### 4.5 Host prerequisites

`terraform` ≥ 1.10, `ssh`, `ssh-keygen`, `git`. The AWS CLI is **not** required. Documented in the README alongside `limactl`, `docker`, and `nono`.

### 4.6 AWS SDK dependencies

`aws-sdk-go-v2` `config`, `sts`, `s3`, and `ec2`. No `ssm` client — the AMI lookup is a Terraform data source.

---

## 5. Non-Functional Requirements

### 5.1 Security

| Control | Requirement |
|---|---|
| Network exposure | SSH ingress restricted to the detected host `/32`. `0.0.0.0/0` must never be written, including on any error path. |
| Egress | Unrestricted outbound, required for apt, npm, GitHub, and the Anthropic API. |
| Host-key verification | `accept-new` against a dedicated `known_hosts`. A changed key on a known host must fail. |
| Data at rest | Root EBS volume encrypted. State bucket encrypted (`AES256`) with public access blocked and versioning enabled. |
| Credential file mode | `~/.claude/.credentials.json` on the instance at `0600`. |
| Private key mode | `~/.isolarium/ec2/id_ed25519` at `0600`. |
| GitHub token handling | Short-lived installation token, passed in a clone URL for a single command, never written to instance disk. |
| Credential destruction | `destroy` terminates the instance; `delete_on_termination` removes the volume holding the refresh token. |
| Disclosure | README must state that a Claude refresh token is placed on the instance. |

### 5.2 Performance budgets

| Operation | Budget |
|---|---|
| `create` end to end | ≤ 12 minutes; SSH reachable ≤ 5 minutes; `cloud-init status --wait` ≤ 15 minutes before timeout |
| `run` / `shell` connect (warm) | ≤ 2 seconds to first byte, dominated by SSH handshake |
| `run` credential check | ≤ 300 ms added per invocation (one SSH read) |
| `status` per EC2 environment | ≤ 1 second (one `DescribeInstances`) |
| `destroy` | ≤ 5 minutes |

`create` cold-start of several minutes is an accepted trade-off against maintaining a Packer pipeline, and must be documented.

### 5.3 Reliability

- All Terraform operations use `-lock-timeout=120s`; a concurrent invocation waits rather than failing.
- An interrupted `destroy` is safe to retry and converges.
- An interrupted `create` leaves `instance-<name>.tf` on disk; re-running `create` fails on the existing-file check, and `destroy` cleans up. This is stated in the error message.
- Stale locks after a killed process are cleared with `terraform force-unlock <id>`, documented in the README.
- SSH connect failure triggers exactly one metadata-refresh retry — never an unbounded loop.

### 5.4 Observability

Progress messages to stdout mirror the Lima flow's style (`Creating EC2 instance...`, `Cloning repository...`). Terraform stdout/stderr is streamed through so the user sees the plan and apply. No pricing or cost data is emitted anywhere.

### 5.5 Testability

- Unit tests must run with no AWS access, no network, and no `terraform` binary, via the injectable function fields in 4.1 and the existing `command.NewFakeRunner`.
- Integration tests use the `//go:build integration` tag and are gated on `ISOLARIUM_EC2_INTEGRATION=1` plus AWS credentials.
- Per the CLAUDE.md test-integrity rule, a gated test that cannot run must **fail with a clear message**, not skip. `test-scripts/test-ec2-integration.sh` must exit non-zero if `go test` reports "no tests to run".

---

## 6. Scenarios and Workflows

### 6.1 Primary end-to-end scenario

**A long-running agent session survives a laptop sleep.**

```
$ isolarium create --type ec2 --name my-work
  → bucket ensured, scaffolding extracted, keypair generated
  → IP detected, instance-my-work.tf written, terraform apply
  → SSH reachable, cloud-init complete
  → repo cloned, git author configured, project config copied
  → creation and post-creation scripts run

$ isolarium run -i --type ec2 --name my-work -- claude
  → credentials written (instance had none)
  → ssh -t ... tmux new-session -A -s isolarium -- claude

  [user closes the laptop; SSH connection drops; claude keeps running]

$ isolarium run -i --type ec2 --name my-work -- claude
  → notice: attaching to existing session 'isolarium'
  → credentials NOT overwritten (instance refreshed; its expiresAt is later)
  → user is back in the same claude session, context intact

$ isolarium destroy --type ec2 --name my-work
  → instance terminated, known_hosts entry evicted, metadata removed
```

This single thread exercises the state-backend bootstrap, Terraform scaffolding, IP detection, keypair generation, instance creation, cloud-init wait, SSH transport, repo clone, conditional credential write, tmux create-and-reattach, and destroy. It is the natural spine for a later steel-thread plan.

### 6.2 Supporting scenarios

| # | Scenario | Expected behavior |
|---|---|---|
| 1 | First-ever `create` in a fresh AWS account | Bucket, VPC, SG, keypair, and instance all created in one flow. No bootstrap command. |
| 2 | Second `create` with a different `--name` | Plan shows `1 to add`; shared infra reused. |
| 3 | `create` while another `create` runs | Second waits up to 120s on the state lock, then proceeds. |
| 4 | Network switch, then any create or destroy | Shared ingress rule re-points to the new `/32`; all instances regain access. |
| 5 | `run` after the instance was stopped and started out of band | SSH connect fails, metadata refreshed via `DescribeInstances`, retry succeeds. |
| 6 | `run -- bash` while `claude` runs in the session | Notice printed; user is attached to the existing session. |
| 7 | `run -i --new-session -- bash` | New session `isolarium-2`; the `claude` session is untouched. |
| 8 | Non-interactive `Exec` | No tmux, no `-t`; exit code propagates faithfully. |
| 9 | `destroy` interrupted mid-apply | Re-running `destroy` converges and completes. |
| 10 | `destroy` while offline from `checkip` but reachable to AWS | Warns, uses persisted CIDR, completes. |
| 11 | `ec2 wipe` with instances present | Refuses; lists the environments to destroy first. |
| 12 | `ec2 wipe` with none present | Tears down shared infra; reports the retained bucket by name. |
| 13 | `terraform` 1.9 installed | Actionable version error before any AWS call. |
| 14 | `AWS_REGION` unset | Fail fast with a clear message. |
| 15 | `--name` `My_Env` for `ec2` | Rejected with the naming rule stated. |
| 16 | `status` with credentials absent | EC2 rows show `unknown`; other backends unaffected. |
| 17 | Rendered `user_data` exceeds 16 KB | `create` fails before apply, naming the limit. |

---

## 7. Constraints and Assumptions

### 7.1 Constraints

- Terraform ≥ 1.10 is a hard prerequisite (S3 native locking).
- One shared state file; all instance operations serialize on one lock.
- One AWS account and one region per host installation. Changing `AWS_REGION` targets a different bucket and a different, independent set of infrastructure.
- Instances are always x86_64 `t3.large`; no override.
- Uncommitted host changes are not carried into the instance.
- The instance is reachable on the public internet on port 22, mitigated by the `/32` rule.

### 7.2 Assumptions

**A1 — Claude Code on Linux refreshes its own access token.** Assumed: Claude Code reads `refreshToken` from `~/.claude/.credentials.json`, refreshes when the access token expires, and rewrites the file. The user explicitly directed that this be assumed (Q31).

*This assumption is load-bearing and unverified.* If false, sessions die within hours, Q6's limitation is real, and an explicit refresh mechanism — a host-side push into the running session, or an on-instance agent — becomes a v1 requirement, materially changing the credential design.

**Required verification, executable today against the `vm` backend in minutes, no EC2 needed:**
1. In a Lima VM with working credentials, set `claudeAiOauth.expiresAt` to a past timestamp, leaving `refreshToken` intact.
2. Run `claude` and issue one prompt.
3. Confirm `accessToken` changed and `expiresAt` moved into the future.

This should be executed before implementation of 3.11 begins.

**A2 — Access tokens have a fixed TTL.** This is what makes the `expiresAt` ordering in 3.11 a valid proxy for "which side refreshed more recently." **Defined fallback if false:** write only when the instance's credentials are unusable — absent, unparseable, or `expiresAt` already in the past — dropping the host-versus-instance comparison.

**A3 — Refresh-token rotation behavior is unknown.** The conditional write in 3.11 is correct under both rotating and non-rotating schemes; it narrows the collision window rather than eliminating divergence.

**A4 — `https://checkip.amazonaws.com` is reachable and returns the address AWS sees.** A user behind a proxy that egresses differently than their SSH traffic would get an unusable rule. Out of scope to detect.

**A5 — Ubuntu 24.04 amd64 supports the full toolchain** — Node LTS, `gh`, rootless Docker, SDKMAN/Temurin, uv, tmux.

### 7.3 Accepted trade-offs

- **Provisioning duplication.** Two copies of the toolchain definition that will drift. Accepted only as an explicit, recorded v1 trade-off against the CLAUDE.md "Pattern-Based Fixes" rule; unification is a named follow-up (8.2).
- **Forgotten instances cost money silently.** No idle auto-stop and no cost messaging. A forgotten `t3.large` plus 50 GiB gp3 runs roughly $64/month with no in-product reminder at any point.
- **Architecture divergence.** Apple Silicon users run x86_64 remotely, so arch-sensitive behavior can differ from local.
- **First-connection TOFU.** Mitigated by the `/32` ingress.
- **Shared-state contention.** Mitigated by `-lock-timeout=120s`.
- **A refresh token lives on a public-facing host** for as long as the instance runs.

---

## 8. Out of Scope

### 8.1 Out of scope for v1

- Idle auto-stop or any cost-control automation.
- Any in-product cost reporting, pricing data, uptime column, or accrued-cost column.
- Instance-size, architecture, or volume-size overrides.
- A dedicated update-ingress command — unnecessary, since every apply re-points the shared rule.
- On-instance token-refresh daemon.
- Pre-warmed instance pools or Packer-built AMIs.
- SSM Session Manager or EC2 Instance Connect Endpoint.
- Multi-region or multi-account orchestration.
- Carrying uncommitted host changes into the instance.
- Automated deletion of the S3 state bucket.
- An IAM instance profile (see 3.1).
- Back-porting the conditional credential write of 3.11 to the `vm`, `container`, and `nono` backends, which clobber unconditionally today.

### 8.2 Named follow-ups

1. **Unify provisioning** between `internal/lima/template.yaml` and `internal/ec2/cloud-init.yaml`. Lowest-risk path: keep `template.yaml` authoritative and have the EC2 path unmarshal it and lift `.provision[].script`, requiring no change to the Lima codepath.
2. **Evaluate back-porting the conditional credential write** to the other three backends, per the pattern-based-fixes rule.
3. **Reconsider cost visibility** if forgotten instances prove painful. Cheapest addition is a `status` uptime column — `DescribeInstances` already returns `LaunchTime`.

---

## 9. Acceptance Criteria

The capability is complete when all of the following hold.

**Functional**

1. `isolarium create --type ec2 --name <n>` succeeds in an AWS account with no prior isolarium infrastructure, creating the state bucket, shared infrastructure, and instance in one command with no bootstrap step.
2. `isolarium run --type ec2 --name <n> -- <cmd>` executes non-interactively and propagates the exit code faithfully.
3. `isolarium run -i --type ec2 --name <n> -- claude` starts a tmux session; after the SSH connection is severed, re-running attaches to the same session with the process still running.
4. `isolarium shell --type ec2 --name <n>` opens an interactive shell in `/home/ubuntu/repo` inside tmux.
5. `isolarium status` lists EC2 environments with correct state, repository, and branch.
6. `isolarium destroy --type ec2 --name <n>` terminates the instance, evicts the `known_hosts` entry, and removes host metadata.
7. `isolarium ec2 wipe` refuses while instances exist, and after they are destroyed tears down shared infrastructure and reports the retained bucket by name.
8. `pid.yaml` `ec2` creation scripts, post-creation host scripts, and env scripts all execute.
9. Two environments coexist; destroying one leaves the other running and reachable.

**Correctness and safety**

10. No code path can produce a security-group rule of `0.0.0.0/0`.
11. `create` aborts with a clear message when `terraform` < 1.10, when `AWS_REGION` is unset, when `--name` violates the naming rule, or when rendered `user_data` exceeds 16 KB — each **before** any AWS resource is created.
12. `destroy` completes when public-IP detection fails but a persisted CIDR exists.
13. An interrupted `destroy`, re-run, converges.
14. `run` recovers from a changed public DNS via a single metadata refresh and retry.
15. A `run` against an instance whose `expiresAt` is later than the host's leaves the instance's credentials byte-for-byte unchanged.

**Quality gates**

16. `make` succeeds — the canonical build command, per CLAUDE.md.
17. Unit tests for the EC2 backend pass with no AWS access, no network, and no `terraform` binary installed.
18. `test-scripts/test-ec2-integration.sh` passes against real AWS with `ISOLARIUM_EC2_INTEGRATION=1`, and **fails rather than skips** when the variable or credentials are absent.
19. README documents: the `terraform` ≥ 1.10 prerequisite, required env vars, cold-start latency, that instances bill until destroyed, that a Claude refresh token is placed on the instance, `terraform force-unlock` recovery, manual state-bucket removal, and the nested-tmux prefix-key caveat.
20. Assumption A1 has been empirically verified, or the A2 fallback and an A1 contingency have been implemented.

---

## Change History

### 2026-08-19: Initial specification

Compiled from `ec2-isolation-type-idea.md` and the 32-question discussion in `ec2-isolation-type-discussion.md`.

Open questions carried into this document from the idea file were resolved as follows:

| Open question | Resolution | Section |
|---|---|---|
| Does `ec2 wipe` refuse or cascade when instances exist? | **Refuses**, listing them. `destroy` is the per-instance verb; a wipe that silently kills running agent sessions is the wrong default. | 3.13 |
| `--new-session` semantics | **Non-destructive** — starts `isolarium-<n>`, never kills. Destroying a running session is what this backend exists to prevent. | 3.6 |
| `ingress_cidr` fallback when detection fails on destroy | Persist every successful detection to `isolarium.auto.tfvars`; fatal on `create`, warn-and-fall-back on `destroy`/`wipe`. | 3.4 |
| Per-instance file and resource naming, and `--name` sanitisation | Validate `--name` against `^[a-z][a-z0-9-]{0,31}$` so it is simultaneously a valid Terraform identifier, filename component, and tag value. **No sanitisation needed.** File `instance-<name>.tf`, resource `aws_instance.<name>`. | 3.8 |
| cloud-init `user_data` 16 KB limit | Validate rendered size host-side; fail before apply with an explicit error. | 3.7 |
| Default `--name` for `ec2` | `isolarium-ec2`, as `defaultEC2Name`. | 3.8 |
| Verify the token-refresh assumption | Recorded as assumption **A1** with an executable verification procedure and a stated contingency; verification required before implementing 3.11. | 7.2 |
| Fixed access-token TTL | Recorded as assumption **A2** with a defined fallback rule. | 7.2 |
| Back-port conditional write to other backends | **Out of scope for v1**; named follow-up. | 8.1, 8.2 |

Deviations from the discussion, with rationale:

- **No IAM instance profile.** Q19 listed `iam.tf` in the scaffolding. The instance makes no AWS API calls — the AMI is resolved by a Terraform data source using the user's credentials, and SSM is out of scope — so an instance profile with no policies would be dead weight. Adding one later is additive.
- **Per-instance Terraform outputs.** Q20 noted that a shared `output` block cannot be appended to from multiple files. Resolved by declaring `instance_id_<name>` and `public_dns_<name>` inside each generated file.
