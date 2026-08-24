# EC2 Isolation Type

## Summary

Add a fourth isolation backend, `ec2`, that runs the isolated environment on an Amazon EC2 instance instead of a local Lima VM, local Docker container, or local nono sandbox. Behaviour and command surface parallel the existing `vm` (Lima) backend: `isolarium create --type ec2` launches a fresh EC2; `isolarium run --type ec2 -- cmd` executes a command on it over SSH; `isolarium destroy --type ec2` terminates it. All AWS infrastructure (VPC, SG, keypair, IAM, EC2 instances) is described by a single Terraform configuration in `~/.isolarium/ec2/terraform/`. The S3 state bucket is created by isolarium directly via the AWS SDK on first run — it exists outside Terraform's lifecycle to break the bucket-for-state chicken-and-egg. State locking uses Terraform's native S3 lockfile, so there is no DynamoDB table.

## Primary goals

1. **Long-running agent sessions that survive laptop sleep / SSH disconnect.** The agent process runs inside a `tmux` session on the EC2; closing the laptop drops the SSH connection but the process keeps running. The next `isolarium run -i` re-attaches.
2. **Access compute larger than the laptop.** Default to a reasonable cloud instance size; "more powerful than your laptop" satisfies the goal for v1.

Cross-platform reach and team-shared infra are not primary v1 drivers but are not actively blocked by these decisions.

## Classification

**Platform / infrastructure capability.** New backend implementing the existing `Backend` interface; no change to user-facing command semantics. The bulk of the work is AWS infrastructure (Terraform configuration, state, credentials, networking) rather than new product workflow.

## Terminology

An **environment** is the `(name, type)` pair. `--name` is a persistent root flag; `--type` is validated in `internal/cli/environment_type.go`. Identity lives on disk at `~/.isolarium/<name>/<type>/`. For the EC2 backend, one environment = one EC2 instance.

## Architecture decisions

| Area | Decision |
|------|----------|
| Lifecycle | `create` = launch fresh EC2; `destroy` = terminate. Mirrors Lima backend. No idle auto-stop in v1. |
| Infra layout | One Terraform working directory at `~/.isolarium/ec2/terraform/`. Stable scaffolding files (VPC, subnet, IGW, route table, base SG **including the shared SSH ingress rule**, shared `aws_key_pair`, instance IAM profile) are embedded via `go:embed` and extracted once. `isolarium create --name foo` writes `instance-foo.tf` containing **only** `aws_instance.foo`, then runs `terraform apply`. All resources share one S3 state file. |
| State backend infra | The S3 state bucket is created **outside** Terraform, via the AWS SDK on first `create` (`CreateBucket`, `PutBucketVersioning`, `PutBucketEncryption`, `PutPublicAccessBlock`). Idempotent. Not managed by `terraform destroy`. Bucket name: `isolarium-tfstate-<account-id>-<region>` (account ID via STS `GetCallerIdentity`). |
| State locking | Terraform native S3 lockfile (`use_lockfile = true`). **No DynamoDB table.** Requires Terraform >= 1.10; isolarium checks the version and fails with an actionable message. Applies pass `-lock-timeout=120s`. |
| Connection | SSH over public IP, key-based, via the **system `ssh` binary** (consistent with every other backend shelling out to a CLI; gives PTY, raw mode, and SIGWINCH handling for free). Public subnet, no EIP. |
| SSH ingress | Isolarium detects the host's public IP via `https://checkip.amazonaws.com` before **every** `terraform apply` and passes `-var="ingress_cidr=<ip>/32"`. A single shared ingress rule on the base SG references it, so any create or destroy re-points ingress for all instances — a network switch self-repairs on the next operation. Failure to detect = fail fast; never falls through to `0.0.0.0/0`. |
| SSH host keys | Dedicated `~/.isolarium/ec2/known_hosts` with `StrictHostKeyChecking=accept-new`. The user's `~/.ssh/known_hosts` is never touched. `destroy` evicts the entry (`ssh-keygen -R`) so AWS recycling a DNS name later doesn't look like an attack. |
| SSH keypair | On first `create`, isolarium generates an Ed25519 keypair host-side. Private half at `~/.isolarium/ec2/id_ed25519` (`0600`). Public half passed as a Terraform variable; Terraform creates the `aws_key_pair` every instance references. |
| Session persistence | One tmux session per instance, fixed name, created/attached with `tmux new-session -A -s isolarium -- <cmd>` over `ssh -t`. Applies to `ExecInteractive` and `OpenShell`; plain `Exec` stays outside tmux. `--new-session` forces a fresh session. Isolarium prints a notice when attaching to an existing session, since tmux silently discards the passed command in that case. |
| Image / provisioning | Latest Ubuntu 24.04 AMI via SSM parameter lookup, provisioned by cloud-init/userdata at boot. **A new `internal/ec2/cloud-init.yaml`, duplicated and adapted from the Lima template rather than shared** — Lima is untouched, at the cost of drift. Same toolchain plus `tmux`. `create` waits for `cloud-init status --wait`. |
| Architecture | **Always x86_64.** One instance type, one AMI path (`/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id`). Broadest image compatibility; no `runtime.GOARCH` branch, no two-arch test matrix. |
| Instance size | Hardcoded `t3.large`, 50 GiB gp3. No user-facing knob in v1. |
| Lookup / state | `create` writes `~/.isolarium/<name>/ec2/metadata.json` (instance ID, public DNS, region, repo owner/repo/branch) from `terraform output`. `run`/`shell` read it and go straight to SSH — **no AWS credentials or `terraform` binary needed on the hot path**. On SSH connect failure, isolarium re-resolves via `DescribeInstances` by the immutable instance ID, rewrites the file, and retries once. |
| Destroy | Remove `instance-<name>.tf`, then `terraform apply`. Symmetric with create, and self-correcting: a resource in state with no config is always planned for destruction, so a failed destroy completes on retry. Also cleans up host metadata and the `known_hosts` entry. |
| Long-lived teardown | `isolarium ec2 wipe` runs `terraform destroy` and removes the local Terraform directory and host keypair. **The S3 state bucket is intentionally retained** — it costs ~nothing, is harmless to reuse, and bootstrap is idempotent. Output says so explicitly; README documents full manual bucket removal. |
| Repo onto EC2 | Clone inside EC2 with a short-lived GitHub App installation token, identical to the Lima flow. Token never lands on disk. `.claude/settings.local.json` and `CLAUDE.md` copied after clone. |
| Claude Code credentials | **Conditional copy.** `run` reads `~/.claude/.credentials.json` from the instance and writes the host blob only if that file is absent/unparseable or the host's `claudeAiOauth.expiresAt` is strictly later. This protects a long tmux session that refreshed mid-flight, while still repairing an instance whose refresh lineage the host rotated out. Diverges from Lima's unconditional write; costs one SSH read per run. |
| Credential lifetime | **Assumed** (not yet verified): Claude Code on the instance refreshes its own access token from `refreshToken` and rewrites the credentials file, so the binding constraint is `refreshTokenExpiresAt` (weeks), not `expiresAt` (hours). If this proves false, an explicit refresh mechanism returns as a v1 requirement. See discussion Q31 for the verification procedure. |
| Credential exposure | The copied Keychain blob **contains `refreshToken`** — a long-lived credential to the user's Claude subscription. Already true of the vm/container/nono backends, but EC2 places it on a public-internet-reachable host that may run for weeks. Mitigations: `0600` file mode, **EBS root-volume encryption**, `DeleteOnTermination` on destroy, /32 SG ingress, and an explicit README statement. |
| AWS credentials/region | Env vars in `.env.local` (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN` optional, `AWS_REGION` required). Consumed natively by Terraform and the AWS SDK. No isolarium-specific profile knob. |
| Terraform integration | Shell out to the `terraform` CLI (new prerequisite, >= 1.10). |
| Tagging | `ManagedBy=isolarium` via provider `default_tags`, plus `Name=<--name>` on the instance. Nothing queries by tag — Q20 looks up by instance ID — so tags are for console visibility and cost allocation only. |
| Cost visibility | **None.** No pricing embedded in the tool, no uptime or accrued-cost columns. README states that instances bill until destroyed. |
| `isolarium status` | Extended to report EC2 instance state (running / stopped / none) alongside other backends. Add `"ec2"` to `knownTypes` and a case in `populateTypeSpecificFields`. |
| Post-creation scripts | Parity with VM backend: `pid.yaml` `host_scripts` and `env_scripts` honored, sharing the existing `envscript` and `hostscript` packages. |
| Integration tests | Real-AWS end-to-end test gated on env vars (e.g. `ISOLARIUM_EC2_INTEGRATION=1` + AWS creds). Must fail (not silently skip) when run as part of a suite claiming to test EC2. |

## Out of scope for v1

- Idle auto-stop / cost-control automation. User is responsible for `destroy`.
- Any in-product cost reporting or pricing data.
- Per-project or per-developer instance-size or architecture overrides.
- A dedicated update-ingress command — largely unnecessary, since every apply re-points the shared ingress rule.
- On-instance token refresh daemon / refresh-token storage on EC2.
- Pool of pre-warmed instances or custom Packer-built AMIs.
- SSM Session Manager / Instance Connect Endpoint as connection method.
- Multi-region or multi-account orchestration.
- Carrying uncommitted host changes into the EC2 (same constraint as the VM backend).
- Automated deletion of the S3 state bucket (retained by `wipe`; manual removal documented).

## Known trade-offs accepted

- **Provisioning duplication.** Two copies of the toolchain definition (Lima template + EC2 cloud-init) that will drift. Unifying them is a named post-v1 follow-up; parsing `.provision[].script` out of the embedded `template.yaml` is the lowest-risk path.
- **Forgotten instances cost money silently.** No auto-stop and no cost messaging means a forgotten `t3.large` runs ~$64/month with no in-product reminder.
- **Architecture divergence.** Apple Silicon users run x86_64 on EC2, so arch-sensitive behaviour can differ from local.
- **First-connection TOFU.** `accept-new` leaves the initial connection unverified, mitigated by the /32 SG ingress.
- **Shared-state contention.** All instance operations serialize on one state lock; `-lock-timeout=120s` makes a second invocation wait rather than fail.
- **A refresh token lives on the EC2 instance.** Long-lived subscription credential at rest on a public-facing host, mitigated but not eliminated by encryption and /32 ingress.
- **Token-refresh behaviour is assumed, not verified.** The whole credential design rests on Claude Code refreshing itself on Linux; this must be confirmed before or during the spec.

## Open questions deferred to spec

- Whether `isolarium ec2 wipe` refuses when instances still exist (safer, and consistent with `destroy` being the per-instance verb) or destroys them as part of the run.
- `--new-session` semantics: start a second differently-named session (non-destructive, likely default) vs kill and replace.
- Fallback for `ingress_cidr` when IP detection fails during `destroy`, so teardown is never blocked — proposal: persist the last-detected value in `isolarium.auto.tfvars` and fall back to it on destroy only.
- Exact naming of the generated per-instance file and resource address (`instance-<name>.tf` / `aws_instance.<name>`), including sanitisation of `--name` into a valid Terraform identifier.
- Whether cloud-init `user_data` stays under the 16 KB limit as provisioning grows, and the fallback if not (push the script over SSH after boot).
- Default `--name` for the `ec2` type, mirroring `isolarium-container` / `isolarium-nono` in `internal/cli/cmd_create.go`.
- **Verify the Q31 assumption** that Claude Code on Linux refreshes from `refreshToken` and rewrites `~/.claude/.credentials.json`. Testable today against the `vm` backend in minutes; if false, a refresh mechanism becomes a v1 requirement and the credential design changes substantially.
- Whether access tokens have a fixed TTL, which is what makes the `expiresAt` ordering meaningful. If not, fall back to writing only when the instance's credentials are unusable.
- Whether the same conditional-write policy should be back-ported to the vm/container/nono backends, since they clobber unconditionally today.
