# EC2 Isolation Type — Discussion

This file records the Q&A used to refine the EC2 isolation type idea.

## Initial Idea (verbatim)

- Implement an ec2 isolation type
- Similar to the `vm` isolation type, but instead of using a local VM, it would use an EC2 instance.
- It would use terraform to create a VPC, the EC2 instances in that VPC and any other supporting EC2 resources
- The VPC is 'long-lived' where as the EC2 are created/destroy similar to the `vm` isolation type
- The terraform state would be in an S3 bucket
- isolarium would ssh into EC2 instance to execute the command
- Ideally the command execution would continue even if the user's laptop sleeps etc
- There needs to be a mechanism to refresh the Claude Code interactive session token in the EC2 instance for longer running sessions

## Codebase context observed

- `Backend` interface in `internal/backend/backend.go` defines `Create`, `Destroy`, `Exec`, `ExecInteractive`, `OpenShell`, `GetState`, `CopyCredentials`.
- Three existing backends: `LimaBackend`, `DockerBackend`, `NonoBackend`.
- VM backend provisions Ubuntu 24.04 via Lima with Docker (rootless), Node.js, gh CLI, SDKMAN/Java/Gradle, Claude Code, uv.
- Repo cloning in VM uses a short-lived GitHub App installation token; clone runs *inside* the VM via `limactl shell` so the token never lives on disk.
- Project config (`.claude/settings.local.json`, `CLAUDE.md`) is copied from host to VM after clone.
- Post-creation scripts (host and env) are supported via `pid.yaml`.

## Q&A

### Q1. Primary motivation

**Question:** What is the primary motivation for the EC2 isolation type?

**Answer:** Multiple goals — primarily (1) **survive laptop sleep/disconnect** so long-running agent sessions don't die, and (2) **access more powerful compute** than the laptop has. Cross-platform reach and team-shared infra are not primary drivers at this stage.

**Implications:**
- Session-persistence mechanism (tmux/screen/SSM/mosh) is a first-class concern, not an afterthought.
- Instance-type selection must be configurable; default should be reasonable but not the design center.
- Cost controls (idle auto-stop, explicit destroy) matter because powerful instances are not free.

### Q2. EC2 instance lifecycle

**Question:** How should EC2 instance lifecycle map to isolarium's create/destroy model?

**Answer:** Mirror VM — `isolarium create` launches a fresh EC2, `isolarium destroy` terminates it.

**Implications:**
- Same mental model as `vm` backend; no new commands needed for v1.
- No idle auto-stop in v1 — user is responsible for `destroy` to stop the bill. (We can revisit if it becomes a footgun.)
- No instance reuse; each `create` starts from a fresh AMI + provisioning.
- Need to make sure `destroy` is robust (also cleans up associated resources: EBS, ENI, SG, etc., if Terraform-managed per-instance).

### Q3. Long-lived AWS infrastructure ownership

**Question:** How should the long-lived AWS infrastructure (VPC, S3 backend, IAM, etc.) be provisioned and owned?

**Answer:** A separate **bootstrap** command (e.g. `isolarium ec2 bootstrap`) runs the long-lived Terraform once per AWS account/region. `create` only manages per-EC2 resources.

**Implications:**
- Clear separation: long-lived "platform" Terraform vs short-lived "session" Terraform.
- Two distinct Terraform root modules (or one module with workspaces) — `bootstrap` and `instance`.
- Bootstrap is idempotent; safe to re-run.
- Need an inverse `isolarium ec2 teardown` command to delete the long-lived infra when the user is done with the whole feature. (Or accept that this is a manual `terraform destroy`.)
- Bootstrap probably also creates the S3 bucket + DynamoDB lock table that the *per-instance* Terraform uses for state — chicken-and-egg note: bootstrap's own state has to live somewhere (local, or a pre-existing bucket the user provides).

### Q4. Connection method

**Question:** How should isolarium connect to the EC2 instance to execute commands?

**Answer:** SSH over public IP, key-based.

**Implications:**
- Per-EC2 Terraform creates the instance in a **public subnet** with a public IP (or attaches an EIP for stability across stop/start, though we said terminate on destroy so EIP isn't strictly required).
- Security group restricts SSH ingress — sensible default is "current user's public IP /32" detected at create time; configurable to a CIDR list.
- SSH key management: isolarium needs to generate or accept a keypair, push the public key to the instance (via Terraform / cloud-init `users` block), and store the private key somewhere on the host (e.g. under `~/.isolarium/ec2/` with `0600`).
- This keeps connection semantics close to `limactl shell` — exec, interactive exec, shell, file copy all map to `ssh`/`scp` calls.
- Trade-off accepted: instance is reachable on the public internet (port 22), mitigated by SG. We are not using SSM in v1.

### Q5. Session persistence across laptop sleep / SSH disconnect

**Question:** How should an interactive `isolarium run -i -- claude` session survive a laptop sleep / SSH disconnect?

**Answer:** **tmux on the EC2 side**, isolarium attaches/re-attaches.

**Implications:**
- `ExecInteractive` for EC2 wraps the command in `tmux new-session -A -s <session-name> -- <cmd>` so it creates-or-attaches.
- A deterministic session name (e.g. `isolarium` or per-command hash) lets re-runs find the right session.
- Need UX for "an existing tmux session is running here — attach, kill, or start new?". Probably: default to attach; provide a flag (`--new-session`) to force a new one.
- tmux must be present on the AMI / installed at provisioning.
- Non-interactive `Exec` does not need tmux — only `ExecInteractive` (and probably `OpenShell`).
- Sleep survival is *for the running process*, not for the SSH connection itself. SSH `ServerAliveInterval`/`ClientAliveInterval` should be tuned so dead connections are detected and torn down, but the process keeps running inside tmux on the server.

### Q6. Claude Code token refresh on long-running EC2 sessions

**Question:** How should the Claude Code session token be refreshed on the EC2 instance during long-running sessions?

**Answer:** No special refresh mechanism in v1 — rely on the existing per-`run` credential copy. Each `isolarium run` reads the current credentials from the host (Keychain on macOS, via `claude.ReadCredentialsFromKeychain`) and writes them into the instance's `~/.claude/.credentials.json`. If a session outlives the token, the user re-invokes `run` (or attaches a new session) to refresh.

**Implications:**
- EC2 backend implements `CopyCredentials` the same way Lima does (write file via SSH, `chmod 600`).
- No on-instance daemon, no refresh-token storage on EC2 — minimal new attack surface.
- Acknowledged limitation: an interactive tmux session that runs longer than the access-token lifetime will hit auth failures inside the session. We accept this for v1 and revisit if it becomes painful in practice.

### Q7. Repo onto EC2

**Question:** How should the EC2 instance get the repository onto its disk?

**Answer:** Clone inside EC2 with a short-lived GitHub App token (mirror VM flow).

**Implications:**
- Reuses the existing GitHub App token-minting code; no new secret-handling code paths.
- `git clone` runs inside the instance via SSH so the token never lands on the EC2 disk (passed in the URL during one command).
- Project config files (`.claude/settings.local.json`, `CLAUDE.md`) get copied to the EC2 the same way they're copied to Lima (via SSH/scp).
- Trade-off accepted: uncommitted host changes are NOT carried into EC2 — same constraint the VM backend has.

### Q8. EC2 image / provisioning

**Question:** How should the EC2 image / provisioning be handled?

**Answer:** Stock Ubuntu 24.04 AMI + cloud-init / userdata at boot.

**Implications:**
- The provisioning script is materially the same as `internal/lima/template.yaml`'s `provision` blocks: apt-installs, Docker rootless, Node.js, gh, SDKMAN/Java/Gradle, Claude Code, uv. Plus `tmux` (new requirement from Q5).
- Cold-start latency is several minutes — acceptable trade-off vs. maintaining a Packer pipeline. Document this.
- The Lima provisioning content should be factored so VM and EC2 backends share the same source-of-truth wherever practical (one provisioning bundle, two delivery mechanisms).
- `create` must wait until cloud-init reports "done" before declaring success (and before attempting clone). Polling `cloud-init status --wait` via SSH is the obvious approach.
- AMI ID lookup: query the latest Ubuntu 24.04 AMI for the chosen region/arch at create time (SSM parameter `/aws/service/canonical/ubuntu/server/24.04/...`) — no hardcoded AMI IDs.

### Q9. AWS credentials and region

**Question:** How should AWS credentials and region be supplied?

**Answer:** **Env vars in `.env.local`** — same pattern as `GITHUB_APP_ID` / `GITHUB_APP_PRIVATE_KEY_PATH`.

**Implications:**
- Expected variables (working set; names to confirm during spec): `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN` (optional), `AWS_REGION`. Isolarium loads `.env.local` already; Terraform and the AWS SDK both consume these env vars natively, so no wiring code is needed.
- Users who already use SSO/`aws configure` can `export` the env-var form (e.g. `aws configure export-credentials`) into `.env.local` or their shell.
- No isolarium-specific profile-name knob — keeps the config surface small.
- Region is required; no implicit default. Fail fast with a clear error if `AWS_REGION` is missing.

### Q10. Terraform integration style

**Question:** How should isolarium invoke Terraform?

**Answer:** **Shell out to the `terraform` CLI**.

**Implications:**
- `terraform` becomes a new prerequisite (alongside `limactl`, `docker`, `nono`). Document in README.
- Two Terraform root modules: one for `bootstrap` (long-lived), one for the per-EC2 instance.
- Module sources live inside isolarium (embedded with `go:embed` and extracted to a temp dir at runtime, mirroring how `internal/lima/template.yaml` is embedded). Users can also `terraform apply` them by hand if needed.
- Isolarium parses `terraform output -json` to get the EC2 public DNS / IP, SSH key path, etc.
- Per-EC2 state lives in the S3 bucket created by bootstrap; key includes the environment name (`--name`) so multiple EC2 environments can coexist.
- Bootstrap state lives locally (in `~/.isolarium/ec2/bootstrap/`) since we can't store bootstrap state in a bucket it hasn't created yet.

### Q11. EC2 instance type and disk size

**Question:** How should the EC2 instance type and disk size be chosen?

**Answer:** Hardcoded sensible default, no knob in v1.

**Implications:**
- Pick one default (proposed: `t3.large` x86 / `t4g.large` arm64, 50 GiB gp3 to match the VM template). Confirm in spec.
- Arch follows the running host's arch (so the AMI lookup and tooling stay consistent), unless we explicitly decide EC2 is always x86.
- No config surface, no override flag. If a user needs different sizing they edit the Terraform module locally. Trade-off accepted: zero ergonomic config for an explicit cost-conscious user.
- "Access more powerful compute" goal (Q1) is partially satisfied by "the default is bigger than your laptop"; tuning is deferred.

### Q12. Multiple environments / naming

**Question:** How should the EC2 backend handle multiple environments?

**Answer:** A single Terraform workspace (one root module). `--name` identifies the EC2 instance within that workspace.

**Implications:**
- One per-instance Terraform root module. `--name` is passed as a Terraform variable (`-var="name=<name>"`).
- Per-instance state in S3 is keyed by name: `terraform init -backend-config="key=instances/<name>.tfstate"`. Each instance has its own state file in the shared bucket, so creates/destroys for different names don't contend on a single state.
- Resource names inside the module (EC2 tag `Name`, SG name, key pair name, etc.) embed `--name` so multiple concurrent instances don't collide in AWS.
- Listing all isolarium-managed EC2s for `isolarium status` becomes "list objects under `instances/` in the S3 bucket" or "describe EC2 instances with the isolarium tag" — to decide in spec.

### Q13. V1 scope

**Question:** What's the scope for v1?

**Answer:** All four:
1. Bootstrap + create + destroy + run + shell (core happy path, parity with Lima backend).
2. `isolarium status` includes EC2.
3. Post-creation scripts (parity with VM): host-scripts and env-scripts from `pid.yaml`.
4. Integration tests against real AWS (opt-in, gated on env vars; mirrors the existing Lima integration-test pattern).

**Implications:**
- This is a large v1. The steel-thread will need to sequence the work carefully — bootstrap and create-then-SSH-and-run-a-trivial-command are the critical path; status, post-creation scripts, and integration tests can layer on after.
- Integration tests need a way to skip without silently passing (per the project's "test script integrity" rule in CLAUDE.md) — opt-in via an env var like `ISOLARIUM_EC2_INTEGRATION=1` plus required AWS creds; otherwise the test build tag isn't honored or the test fails fast with a clear "missing env" message.
- Post-creation script support means the EC2 backend implements the same `runIsolationScripts` / `runPostCreationScripts` pattern Lima does (`internal/backend/lima_backend.go:65`), sharing the `envscript` and `hostscript` packages.

### Q14. SSH keypair handling

**Question:** How should the SSH keypair be handled?

**Answer:** Bootstrap generates one shared keypair.

**Implications:**
- Bootstrap Terraform generates an Ed25519 keypair, uploads the public half as an AWS keypair, and writes the private half to `~/.isolarium/ec2/id_ed25519` with `0600`.
- All EC2 instances launched against this bootstrap reuse the same keypair (AWS keypair name embedded in the per-instance module via a Terraform `data` lookup or a fixed output).
- The bootstrap state references the keypair; teardown deletes it.
- Key rotation = re-bootstrap (acceptable for v1).

### Q15. SSH ingress scoping

**Question:** How should SSH ingress (security group rule) be scoped?

**Answer:** Detect the host's current public IP at `create` time, allow only that /32.

**Implications:**
- At `isolarium create --type ec2`, isolarium calls `https://checkip.amazonaws.com`, parses the IP, and passes it as a Terraform variable on the `terraform apply` command line (e.g. `-var="ingress_cidr=<ip>/32"`). The detection happens host-side, not inside Terraform — keeps the module deterministic / portable and removes any need for Terraform to make external HTTP calls.
- The per-instance Terraform module writes that /32 into the SG ingress rule.
- If the user's IP changes (Wi-Fi switch, VPN flip), SSH will fail. v1 remedy is `isolarium destroy && isolarium create`. We can add an "update ingress" command later if it bites.
- Failure to detect the IP (no internet, IP service down) should fail fast with a clear error, not silently fall through to 0.0.0.0/0.

### Q16. Classification

**Question:** How should this idea be classified?

**Answer:** **C — Platform / infrastructure capability.**

**Rationale:**
- Adds a new backend (`EC2Backend`) parallel to the existing `LimaBackend`, `DockerBackend`, `NonoBackend` against the same `Backend` interface (`internal/backend/backend.go:14`). No new user-facing commands beyond a `bootstrap` setup step.
- Introduces substantial new infrastructure: Terraform modules (bootstrap + per-instance), S3-backed remote state, VPC, IAM, AWS credentials handling, public-IP detection. This is the platform character of the work.
- User-facing semantics are unchanged: `isolarium create --type ec2`, `isolarium run --type ec2 -- ...`, `isolarium destroy --type ec2`, same flags. The capability is the new value, not a new workflow.
- Architectural POC elements (does tmux survive sleep cleanly? is cloud-init provisioning reliable enough? are cold-start times tolerable?) exist but are validated within the platform delivery, not as a separate validation exercise.
- Steel-thread implication: incremental delivery of a platform capability — bootstrap first, then minimal `create` + SSH + trivial command, then layer in clone/credentials/post-creation/status/integration tests.

### Q17. Anything else before moving to the specification?

**Question:** Are there any additional requirements or concerns before we move to the next step (creating the detailed specification)?

**Answer:** No — proceed to the next step (specification).

### Q18. Post-decision clarification — Terraform file layout

**User clarification (after Q17):**
> `~/.isolarium/ec2/terraform` contains terraform files. Creating an ec2 instance adds a file here and runs `terraform apply`.

**Refined model (supersedes earlier "per-name state key" thinking in Q10 and Q12):**
- `~/.isolarium/ec2/bootstrap/` — long-lived bootstrap module (VPC, S3 bucket, DynamoDB lock table, IAM, shared keypair). State is local.
- `~/.isolarium/ec2/terraform/` — the per-instance Terraform working directory, persistent on disk. Contains stable files (provider, backend config pointing at the bootstrap-created S3 bucket, variables, shared data sources for VPC/subnet/SG/keypair from the bootstrap state via `terraform_remote_state`) plus one generated file per instance:
  - `isolarium create --name foo` writes `instance-foo.tf` declaring an `aws_instance.foo` (plus its SG ingress rule and any other per-instance resources), then runs `terraform apply`.
  - `isolarium destroy --name foo` deletes `instance-foo.tf` and runs `terraform apply` (or `terraform destroy -target=aws_instance.foo` then removes the file).
- A single shared state file in S3 holds all per-instance resources. `isolarium status` can read this state to enumerate instances.

**Implications:**
- The stable scaffolding files are still embedded in the binary (`go:embed`) but are *extracted once* into `~/.isolarium/ec2/terraform/` on first `create` (or by `bootstrap`), not re-extracted each run. Users can inspect/edit them.
- Per-instance `.tf` files are generated from a template (the public-IP /32, instance type, name tag, etc. are interpolated host-side at file-write time, *not* passed as `-var` for the per-instance bits — only `ingress_cidr` likely still passes as a var since it can change between runs of the same instance).
- Concurrency: all instance ops contend on one state-file lock. Acceptable trade-off; users rarely create two EC2 envs at once.
- This supersedes Q12's note about "state keyed per name in S3". `--name` still identifies the instance, but as a Terraform *resource address* (and tag), not a state-file key.

### Q19. Drop the bootstrap directory; create S3+DynamoDB via AWS SDK

**User clarification (after Q18):**
> Surely 4+5 [the S3 state bucket and DynamoDB lock table] can be created via APIs. Everything else is in a single terraform directory.

**Refined model (supersedes Q3, Q14's bootstrap-keypair note, and Q18's two-directory layout):**
- **No bootstrap Terraform directory and no bootstrap command.** The only reason bootstrap existed was the chicken-and-egg of S3 state's bucket. Removing that obstacle removes the bootstrap.
- **One Terraform directory:** `~/.isolarium/ec2/terraform/`. Contains:
  - Stable scaffolding extracted once from `go:embed`: `provider.tf`, `backend.tf` (S3 backend), `network.tf` (VPC, subnet, IGW, route table), `security.tf` (base SG), `keypair.tf` (the shared `aws_key_pair`), `iam.tf` (instance profile/role), `variables.tf`, `outputs.tf`.
  - Dynamically generated per instance: `instance-<name>.tf` (the `aws_instance.<name>` plus its SG ingress rule with the host's /32).
- **S3 state bucket + DynamoDB lock table** are created by isolarium directly via AWS SDK on first run — a small Go function (CreateBucket + PutBucketVersioning + PutBucketEncryption + CreateTable). Idempotent. Terraform does not manage these resources.
- **Shared SSH keypair**: generated host-side on first run, public half registered as an `aws_key_pair` *via Terraform* (so it's torn down on `terraform destroy` of everything), private half written to `~/.isolarium/ec2/id_ed25519` (`0600`). This is a minor revision of Q14 — the *Terraform* generates the AWS keypair resource; the *private key material* is created by isolarium host-side and passed in as a Terraform variable on first apply.
- **First-run UX (implicit, default):** `isolarium create --type ec2 --name foo` detects whether the S3 bucket and DynamoDB table exist; if not, creates them via SDK; runs `terraform init` (idempotent); writes `instance-foo.tf`; runs `terraform apply`.
- **Resulting layout:**
  ```
  ~/.isolarium/ec2/
    terraform/
      provider.tf
      backend.tf       # backend "s3" {} referencing the SDK-created bucket+table
      network.tf       # VPC, subnet, IGW, route table
      security.tf      # base SG
      keypair.tf       # aws_key_pair using the generated public key
      iam.tf           # instance profile
      variables.tf
      outputs.tf
      instance-foo.tf  # generated per --name; removed on destroy
    id_ed25519         # private key, 0600
    id_ed25519.pub
  ```

**Implications:**
- No `isolarium ec2 bootstrap` / `isolarium ec2 teardown` commands. To wipe everything: `isolarium destroy --name <each>` (terminates instances + tears down VPC/SG/keypair/IAM via the same Terraform on last destroy? — actually no, those persist; they're terraform-managed but referenced by every instance). To wipe long-lived infra a user would `terraform destroy` directly inside `~/.isolarium/ec2/terraform/`, or we add an explicit `isolarium ec2 destroy-all` later. For v1 this is acceptable; mark as an open question in the spec.
- The S3 bucket and DynamoDB table are *unmanaged* by Terraform. They never get destroyed by `terraform destroy`. A future `isolarium ec2 wipe` command (or manual cleanup) handles their teardown.
- Bucket naming needs to be deterministic and account/region-unique: proposed `isolarium-tfstate-<account-id>-<region>`. Isolarium can resolve `<account-id>` via STS `GetCallerIdentity` at first-run time.

### Q20. Connection-detail and state lookup at run/shell/status time

**Question:** At `run`/`shell`/`status` time, how should isolarium learn an EC2 environment's connection details (public IP/DNS, instance ID) and current state?

**Options presented and trade-offs:**

| Option | Latency | Deps on hot path | Freshness | Coupling |
|--------|---------|------------------|-----------|----------|
| A. Local `metadata.json` + SDK `DescribeInstances` for live state | Best — file read + SSH | None for `run`/`shell` (no AWS creds, no `terraform` binary) | Cached DNS can drift if instance stopped/started outside isolarium | Lowest — no Terraform coupling |
| B. `terraform output -json` every command | Worst — Terraform startup + S3 state read (~2–5s) | `terraform` binary **and** AWS creds for every command, incl. `status` | No better than A — TF state is itself a snapshot | Depends on CLI output shape; conflicts with one-file-per-instance (can't append to a shared `output` block from multiple files) |
| C. `DescribeInstances` by tag, no local state | ~200–500ms per command | AWS creds for every command | Authoritative — only option always correct about IP/state | Lowest to Terraform, but repo/branch fields have nowhere to live except AWS tags or an SSH read |
| D. Parse S3 `terraform.tfstate` directly | ~100–300ms (S3 GET) | AWS creds | Same snapshot problem as B | Worst — `tfstate` is an internal format; public interface is `terraform output` / `terraform show -json` |

B and D were assessed as dominated: neither is more accurate than C, and both are slower than A.

**Answer:** **A — local metadata plus SDK state**, with a refresh-on-failure fallback.

**Implications:**
- `create` writes `~/.isolarium/<name>/ec2/metadata.json` from `terraform output`: instance ID, public DNS, region, and repo owner/repo/branch.
- `run`/`shell` read the file and go straight to SSH — no AWS creds and no `terraform` binary required on the hot path.
- `status` reads the file for descriptive fields and calls EC2 `DescribeInstances` (via SDK) for live state.
- **Refresh-on-failure:** the instance ID is immutable, the public DNS is not. If SSH fails to connect, isolarium calls `DescribeInstances` by the cached instance ID, rewrites `metadata.json`, and retries once. This recovers from stop/start or out-of-band `terraform apply` without paying a lookup on every command.
- Matches the existing codebase pattern exactly: Lima writes `metadata.json` for repo/branch (`internal/lima/metadata.go`) and calls `limactl list` for live state. `status.ListAllEnvironments` (`internal/status/environment.go:41`) already walks `<name>/<type>/metadata.json` — adding `"ec2"` to `knownTypes` and an `ec2` case in `populateTypeSpecificFields` is the whole change.
- Testability: metadata read is pure filesystem; the `DescribeInstances` call sits behind one injectable func, mirroring `VMExecFunc` / `ExecFunc` in the existing backends.
- Host-local metadata is not a multi-machine limitation in practice, because the SSH private key and the SG /32 ingress rule are host-local too.

### Q21. When and how the SSH ingress /32 is determined

**Question:** How should the host's public IP /32 reach the security-group ingress rule in the generated Terraform?

**Clarification raised by user:** *"But how/when is the IP address determined?"*

**Mechanism (settled in Q15, not in question):** host-side in Go — HTTPS GET to `https://checkip.amazonaws.com`, parse body, append `/32`. Fails fast if the call fails; never falls through to `0.0.0.0/0`. Deliberately *not* a `data "http"` source inside Terraform, so the module stays deterministic, portable, and makes no external calls of its own.

**Timing options presented:**

| Option | Detection fires when… | Instance foo's SG rule rewritten when… |
|--------|----------------------|----------------------------------------|
| 1. Literal baked into `instance-<name>.tf` | `create --name foo` only | never, after foo is created |
| 2. Shared `-var="ingress_cidr=..."` on every apply | every `terraform apply` — every `create` *and* `destroy`, of **any** name | any create/destroy of any instance |
| 3. Baked in + explicit `isolarium ec2 refresh-ingress` | create, plus explicit refresh | on explicit refresh only |
| 4. Baked in + self-heal on the Q20 SSH-failure path | create, plus on SSH connect failure during `run` | automatically during failed `run` |

Option 4 was noted as folding into the recovery path Q20 already requires, at the cost of making `run` able to mutate infrastructure and reintroducing an AWS-credential dependency on a path Q20 kept credential-free.

**Answer:** **Option 2 — shared `-var`, re-detected on every apply.**

**Implications:**
- Isolarium detects the public IP before *every* `terraform apply`, i.e. on every `create` and every `destroy`, regardless of `--name`, and passes `-var="ingress_cidr=<ip>/32"`.
- **Derived simplification:** because all instances share one CIDR value, per-instance ingress rules are unnecessary. The SSH ingress rule moves into the stable `security.tf` on the base SG, referencing `var.ingress_cidr`. `instance-<name>.tf` therefore contains only the `aws_instance.<name>` resource (root block device, tags, user_data, key pair, SG reference). This is simpler than the Q19 sketch, which put an ingress rule in each generated file.
- **Self-repairing ingress, for free:** switching networks and then creating or destroying *anything* re-points the shared rule at the current IP, un-stranding every existing instance. This largely dissolves the Q15 limitation ("re-create the instance if your IP changes") without a new command — a plain `isolarium destroy --name <throwaway>` or any subsequent `create` repairs access.
- **Accepted cost:** an apply for one instance shows a change to shared infrastructure. With the derived single-shared-rule design this is `1 to add, 1 to change`, not one change per instance, so blast radius is small and the diff is easy to read.
- **Spec-level detail to resolve:** `destroy` also needs an `ingress_cidr` value. If IP detection fails at destroy time (offline, `checkip` blocked but AWS reachable), failing fast would block teardown. Proposal: persist the last-detected CIDR (e.g. in `~/.isolarium/ec2/terraform/isolarium.auto.tfvars`) and fall back to it on `destroy` only, so teardown is never blocked by IP detection.

### Q22. Destroy mechanics

**Question:** How should `isolarium destroy --type ec2 --name foo` remove the instance? (Open question carried from the idea file.)

**Failure-mode analysis presented:**
- *Remove-file-then-apply*: if the apply fails, the config is gone but the resource remains in state. Terraform always plans destruction for a resource in state with no configuration, so a retry self-corrects.
- *Targeted-destroy-then-remove-file*: if the destroy succeeds but the file removal fails, a stale config survives and **the next `create` of any other name silently resurrects the destroyed instance.** This asymmetry was judged more important than declarative purity.

| Option | Blast radius | Failure mode |
|--------|--------------|--------------|
| 1. `rm instance-foo.tf` then `terraform apply` | converges shared-infra drift too | self-correcting on retry |
| 2. `terraform destroy -target=aws_instance.foo` then `rm` | narrowest; leaves shared ingress alone | stale config can resurrect the instance |
| 3. Move file aside, apply, delete backup on success | same as 1 | clean rollback, but needs `*.removing` sweep logic |

**Answer:** **Option 1 — remove the file, then `terraform apply`.**

**Implications:**
- `destroy` is symmetric with `create`: both mutate the working directory and then converge with a single `terraform apply`. One codepath, one mental model.
- The config in `~/.isolarium/ec2/terraform/` is the source of truth; state converges to it.
- A failed or interrupted `destroy` is safe — rerunning it (or any later `apply`) completes the destruction. No orphan-resurrection hazard.
- Consistent with Q21: the same apply re-detects the public IP and re-points the shared ingress rule, so a destroy also repairs ingress for surviving instances.
- Accepted cost: the destroy plan converges any unrelated drift in shared infrastructure. Acceptable because the shared infra is isolarium-managed and small.
- `destroy` must also clean up the host-side `~/.isolarium/<name>/ec2/` metadata directory (per Q20), mirroring `lima.CleanupHostMetadata`.

### Q23. SSH client and host-key verification

**Derived (not asked) — use the system `ssh` binary.** Every existing backend shells out to a CLI (`limactl`, `docker`, `nono`), Terraform adds another shell-out, and `ExecInteractive` must attach to a tmux session — requiring PTY allocation, raw terminal mode, and SIGWINCH window-resize propagation. `ssh -t` provides all of this; Go's `crypto/ssh` would mean hand-writing it. Recorded as settled.

**Question:** How should isolarium verify the EC2 instance's SSH host key? (Sharper than for Lima because AWS recycles public IPs and DNS names between instances, so stale entries genuinely collide.)

| Option | TOFU window | Cost |
|--------|-------------|------|
| 1. Dedicated `known_hosts`, `StrictHostKeyChecking=accept-new` | first connection unverified | must evict entries on destroy or recycled IPs cause mismatches |
| 2. Pre-seed host key via cloud-init `user_data` | none | host private key readable via `ec2:DescribeInstanceAttribute` |
| 3. Verify fingerprint from `ec2:GetConsoleOutput` | none | console output lags boot 2–4 min; brittle parsing |
| 4. `StrictHostKeyChecking=no`, `UserKnownHostsFile=/dev/null` | permanent | no MITM protection at all, on the connection that carries credentials |

**Answer:** **Option 1 — dedicated `known_hosts` with `accept-new`.**

**Implications:**
- All SSH invocations pass `-o UserKnownHostsFile=~/.isolarium/ec2/known_hosts -o StrictHostKeyChecking=accept-new -i ~/.isolarium/ec2/id_ed25519`.
- The user's `~/.ssh/known_hosts` is never touched — no pollution, no churn in a file they care about.
- Trust on first use, but a *changed* key on a known host fails loudly. That is the protection that matters in practice: it catches a redirected connection to an already-known instance.
- **`destroy` must evict the entry** (`ssh-keygen -R <public_dns> -f ~/.isolarium/ec2/known_hosts`), otherwise AWS recycling that DNS name or IP for a later instance produces a host-key-mismatch failure that looks like an attack. This is a required step in the destroy sequence alongside metadata cleanup (Q22).
- Accepted residual risk: the first connection to a new instance is unverified. Mitigated by the /32 SG ingress (Q15/Q21) narrowing who can reach port 22 in the first place.
- The SSH option set should live in one place (a `buildSSHArgs` helper) so `Exec`, `ExecInteractive`, `OpenShell`, and `CopyCredentials` cannot drift apart — mirroring how `internal/lima/ssh.go` centralizes `BuildShellCommand`.

### Q24. Relationship between EC2 provisioning and the existing Lima provisioning

**Context observed in code:** `internal/lima/template.yaml:27` has two provision blocks — `mode: system` (apt essentials, Docker rootless prereqs, the `kernel.apparmor_restrict_unprivileged_userns` sysctl, Node.js LTS, GitHub CLI) and `mode: user` (rootless Docker via get.docker.com, `loginctl enable-linger`, SDKMAN, Claude Code via npm, uv). They are not portable as-is: the user block references `$USER`, which is the host username under Lima but always `ubuntu` on EC2.

**Question:** How should the EC2 provisioning content relate to the existing Lima provisioning?

| Option | Duplication | Risk to Lima |
|--------|-------------|--------------|
| 1. Extract both blocks to `internal/provision/*.sh`, generate `template.yaml` from them | none | refactors a working path — EC2 work carries VM regression risk |
| 2. Duplicate: new EC2-specific cloud-init, Lima untouched | two copies that can drift | none |
| 3. Keep `template.yaml` authoritative; EC2 unmarshals it and lifts `.provision[].script` | none | none, but an unusual indirection |

**Answer:** **Option 2 — duplicate for v1.**

**Implications:**
- New `internal/ec2/cloud-init.yaml` (or a Go-templated equivalent), parallel to `internal/lima/template.yaml`. Lima's template is not touched, so the EC2 feature carries no VM regression risk and the steel thread stays self-contained.
- The EC2 copy adapts the Lima content: root-level steps become `runcmd` (or `packages:`), user-level steps run as `ubuntu` explicitly rather than via `$USER`, and `tmux` is added (required by Q5).
- **Accepted cost — drift.** Two copies of the toolchain definition. A Node.js LTS bump, a new tool, or a version pin must be applied in both places. This sits against the CLAUDE.md "Pattern-Based Fixes" rule, so it is accepted only as an explicit, recorded v1 trade-off.
- **Required follow-up:** unifying the two into a shared source of truth should be captured as a named post-v1 item, not left implicit. Option 3 (parse `.provision[].script` out of the embedded `template.yaml`) is the lowest-risk unification path if it is done later, since it needs no change to the Lima codepath.
- Spec should note the cloud-init `user_data` 16 KB limit. The current script content is well under it, but if provisioning grows, the fallback is to have cloud-init fetch or have isolarium push the script over SSH after boot.
- `create` still waits on `cloud-init status --wait` before declaring success (Q8).

### Q25. Terraform state locking mechanism

**Context raised:** Terraform 1.10 (Nov 2024) added native S3 state locking via `use_lockfile = true`, built on S3 conditional writes (`If-None-Match`, available since Aug 2024). HashiCorp deprecated `dynamodb_table` in Terraform 1.11 and has signalled removal. This postdates the Q19 sketch, which assumed a DynamoDB lock table.

**Question:** How should Terraform state locking work, given Q19 has isolarium create the state backend via the AWS SDK?

| | S3 lockfile | DynamoDB | No locking |
|---|---|---|---|
| Terraform version floor | ≥ 1.10 | none meaningful | none |
| SDK bootstrap | CreateBucket + versioning + encryption + public-access-block | all that **plus** CreateTable **plus** an async waiter (~10–20s, polling loop to write and test) | bucket only |
| IAM surface | `s3:*` on one bucket | adds `dynamodb:GetItem/PutItem/DeleteItem/DescribeTable` | smallest |
| Idle cost | ~$0 | ~$0 if `PAY_PER_REQUEST`; ~$0.65/mo if provisioned | $0 |
| Teardown surface | one resource | two | one |
| Future work | none — supported direction | migration already scheduled by deprecation | none |

**Deciding failure mode:** Q19's single shared state file means `create --name a` and `destroy --name b` contend even though they touch unrelated instances. With either lock, the loser gets a clear `Error acquiring the state lock`. Without a lock, both applies read the same state and last-writer-wins, leaving **orphaned EC2 instances that are running and billing but absent from state and invisible to `isolarium status`**. Recovery requires `terraform import` or console archaeology.

Additionally, "no locking" contradicts Q19, which explicitly accepted "all instance ops contend on one state-file lock" as the price of a single shared state file — that reasoning presupposes a lock exists.

**Answer:** **S3 native lockfile (`use_lockfile = true`), no DynamoDB.**

**Implications — this supersedes the DynamoDB parts of Q19 and the idea file:**
- SDK bootstrap reduces to: `CreateBucket`, `PutBucketVersioning`, `PutBucketEncryption`, `PutPublicAccessBlock`. Idempotent. No `CreateTable`, no table waiter.
- Backend block: `bucket`, `key = "terraform.tfstate"`, `region`, `encrypt = true`, `use_lockfile = true`.
- IAM/credential requirements drop `dynamodb:*`.
- **New prerequisite: Terraform >= 1.10.** Isolarium must detect the installed version and fail with a clear, actionable message rather than surfacing a raw Terraform parse error. This resolves part of the idea file's "exact prerequisite versions" open question.
- Pass `-lock-timeout=120s` on apply so a second concurrent isolarium invocation waits rather than erroring out immediately.
- Stale locks after a killed apply are cleared with `terraform force-unlock <id>` — same UX as DynamoDB. Worth documenting.
- Bucket versioning is retained for state recovery; note that the `.tflock` object is versioned too, which adds harmless noise.

### Q26. Teardown of long-lived infrastructure

**Scope of "long-lived" after Q25:** Terraform-managed VPC, subnet, IGW, route table, base SG (with the Q21 shared ingress rule), `aws_key_pair`, and IAM instance profile — all in the single state file — plus the SDK-created S3 state bucket, which Terraform deliberately does not manage.

**Cost is not the driver.** VPC, subnet, IGW, route table, SG, key pair, and IAM role are all free; a bucket holding a small state file costs fractions of a cent per month. The motivation is account hygiene and clean uninstall, not spend.

**The awkward part is the bucket.** Q25 enables versioning, and `aws s3 rb` refuses a non-empty bucket while `aws s3 rm --recursive` does not remove non-current versions or delete markers. Manual cleanup requires `s3api list-object-versions` piped into `delete-objects`.

| | Manual only | Full `ec2 wipe` | Terraform destroy only |
|---|---|---|---|
| Code in v1 | none | command + confirmation + version-delete loop | thin command wrapper |
| Uninstall UX | multi-step, awkward bucket dance | one command, complete | one command, bucket survives |
| Risk | none | rarely-exercised destructive path | small |

Auto-teardown-on-last-destroy was assessed and rejected: it makes an everyday command occasionally do something drastic, behaves differently depending on whether the named instance happens to be the last one, and can race a concurrent create under Q25's shared lock.

**Answer:** **Terraform destroy only — ship a thin `isolarium ec2 wipe`, leave the state bucket in place.**

**Implications:**
- `isolarium ec2 wipe` runs `terraform destroy` in `~/.isolarium/ec2/terraform/` (tearing down VPC, subnet, IGW, route table, SG, keypair, IAM) and removes the local Terraform working directory.
- The S3 state bucket is **intentionally retained**. It costs ~nothing, is harmless to reuse, and the bootstrap in Q19/Q25 is already idempotent — a later `create` finds the existing bucket and proceeds. This keeps the version-deletion loop out of v1 entirely.
- Output must say plainly that the bucket was left behind, name it, and point at the README for full removal — otherwise "wipe" over-promises.
- README documents the manual `s3api list-object-versions` + `delete-objects` + `delete-bucket` sequence for anyone wanting a truly clean account.
- Open behaviour to settle in spec: whether `wipe` refuses when instances still exist, or destroys them as part of the run. Refusing (with a list of what remains) is the safer default and consistent with `destroy` being the per-instance verb.
- `wipe` should also remove the host-side SSH keypair and `known_hosts` under `~/.isolarium/ec2/` (Q23), since the AWS key pair it corresponds to is gone.

### Q27. tmux session naming and reattach semantics

**Clarification raised by user:** *"What's an environment?"*

**Definition, from the code:** an environment is the `(name, type)` pair. `--name` is a persistent root flag; `--type` is validated against a fixed list in `internal/cli/environment_type.go:13` (`vm`, `container`, `nono` — `ec2` gets added there). Identity is that pair on disk: `status.ListAllEnvironments` (`internal/status/environment.go:41`) walks `~/.isolarium/<name>/<type>/` and emits one `EnvironmentStatus` per directory. Per-type name defaults live at `internal/cli/cmd_create.go:11`. `foo/vm` and `foo/container` are distinct environments sharing a name. **For the EC2 backend, one environment = one EC2 instance**, so the question is how many concurrent tmux sessions may live on one box.

**Derived (not asked):** `OpenShell` gets the same tmux treatment as `ExecInteractive` — a shell is precisely where a long job gets started before the laptop closes. Non-interactive `Exec` stays outside tmux (Q5), since exit codes and stdout are unreliable through a tmux attach.

**Question:** On a given EC2 instance, how should tmux sessions be named and reattached?

| | One per command | One per instance | Per instance + detection |
|---|---|---|---|
| Reconnect to `claude` after sleep | works | works | works |
| `run -- bash` while claude runs | separate session, both alive | silently attaches to claude | prompts clearly |
| Session names | `claude`, `bash`, … | fixed, e.g. `isolarium` | fixed |
| Stale accumulation | one per distinct command | none | none |
| Code | little | least | most; needs `list-sessions` parse + prompt, plus a non-TTY fallback |

**Answer:** **One session per instance** — a single fixed session name per `--name`, with `--new-session` to force a fresh one.

**Implications:**
- `ExecInteractive` and `OpenShell` wrap the command as `tmux new-session -A -s isolarium -- <cmd>`, run over `ssh -t`.
- A session whose command exits disappears on its own, so a finished session never blocks the next run. Collisions only occur while something is genuinely still running.
- **Accepted trap:** `tmux new-session -A` attaches to an existing session and silently discards the command argument. Requesting `bash` while `claude` is running drops the user into `claude` with no explanation.
- **Cheap mitigation worth specifying:** before attaching, isolarium can check whether a session already exists and, if so, print a one-line notice (`attaching to existing session 'isolarium'; use --new-session to start a fresh one`) rather than prompting. This removes the surprise without adding an interactive prompt or a non-TTY fallback path, preserving the simplicity that motivated this choice.
- `--new-session` semantics to settle in spec: start a second, differently-named session, or kill and replace the existing one. Starting a second session is non-destructive and therefore the safer default.
- Nested-tmux caveat: users who run tmux locally will be attaching a remote tmux inside a local one. Worth a README note on the prefix key, not a code concern.

### Q28. EC2 CPU architecture

**Context:** Q11 said arch "follows the running host's arch ... unless we explicitly decide EC2 is always x86", and the idea file carried this as unresolved. Following the host is a Lima inheritance — Lima *must* match the host to avoid emulation. EC2 has no such constraint, so the choice is free.

**Pricing (us-east-1):** `t3.large` ~$0.0832/hr vs `t4g.large` ~$0.0672/hr — Graviton ~19% cheaper.

**Compatibility cuts both ways:** isolarium provisions rootless Docker inside the instance, so whatever the user's repo pulls or builds runs at the instance's architecture. An x86-only base image fails on arm64; conversely an Apple Silicon user could see different behaviour on an x86 instance. Ubuntu 24.04 arm64 supports everything in the provisioning set (Node LTS, `gh`, Docker, SDKMAN/Temurin, uv).

| | Follow host | Always arm64 | Always x86_64 |
|---|---|---|---|
| Matches local dev | yes | Apple Silicon only | x86 hosts only |
| Cost | varies | ~19% cheaper | baseline |
| Image compatibility | same as local | x86-only images fail | broadest |
| Spec/test surface | mapping table + two-arch test matrix | one constant, one AMI path | one constant, one AMI path |

**Answer:** **Always x86_64 — `t3.large`.**

**Implications — this supersedes Q11's "arch follows the host" and the idea file's AMI-architecture row:**
- One hardcoded instance type: `t3.large`. No host-arch mapping table, no `runtime.GOARCH` branch.
- One SSM AMI parameter path: `/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id`. Looked up at create time; no hardcoded AMI IDs (Q8).
- Integration tests need only a single-arch path — no two-arch matrix.
- Broadest compatibility inside the instance: any Docker image, any prebuilt binary, no `exec format error` surprises.
- **Accepted cost:** ~19% more expensive than Graviton, and Apple Silicon users run a different architecture than their laptop, so local-vs-EC2 behaviour can diverge for arch-sensitive work. Worth a README note.
- Storage stays 50 GiB gp3, matching the VM template (Q11).
- Because there is now exactly one instance type, revisiting size or switching to Graviton later is a one-constant change plus the AMI path — cheap to defer.

### Q29. Cost visibility

**Context:** Q2 ruled out idle auto-stop, so nothing but the user's memory stands between a forgotten instance and an ongoing bill. `t3.large` (~$0.0832/hr) plus 50 GiB gp3 (~$4/mo) is roughly **$0.089/hr — ~$2.13/day, ~$64/month if left running**.

**Accuracy caveat:** prices vary by region and the AWS Pricing API is awkward (us-east-1-only endpoint, fiddly filters, extra IAM). Any figure isolarium printed would be a hardcoded approximation or a small region-keyed table, and would drift.

| | Silent | Notice at create | Notice + status accrual |
|---|---|---|---|
| Catches "I forgot to destroy" | no | only if remembered | yes, every `status` |
| New code | none | one formatted line | line + duration/cost formatting |
| Wrongness risk | none | stale hardcoded price | same, shown repeatedly |

**Answer:** **Stay silent** — no cost messaging in the tool.

**Implications:**
- No pricing data is embedded in isolarium. Nothing to drift, no implication that isolarium tracks spend, no Pricing API call or IAM permission on the create path.
- `isolarium status` reports EC2 state only (Q20) — no uptime column, no accrued-cost column.
- README documents plainly that EC2 instances bill until `isolarium destroy` is run, and points at AWS pricing rather than quoting numbers.
- **Accepted risk:** combined with no idle auto-stop (Q2), a forgotten instance costs roughly $64/month with no in-product reminder at any point. This is a deliberate choice to keep the tool free of pricing claims; cost control is entirely the user's responsibility.
- If this proves painful, the cheapest later addition is a status uptime column — `DescribeInstances` already returns `LaunchTime`, so it needs no extra call, tag, or bookkeeping.

### Q30. Tagging strategy (derived, not asked)

The idea file listed tagging as an open question, proposing `ManagedBy=isolarium`, `Name=<--name>`, `CreatedAt=<ts>`. This is now derivable from decisions already made:

- **Tags are not functionally required.** Q20 chose local `metadata.json` plus `DescribeInstances` *by instance ID* for lookup. Nothing in isolarium ever queries by tag, so tags exist purely for human visibility in the AWS console and for cost allocation.
- **`CreatedAt` is redundant.** `DescribeInstances` returns `LaunchTime`, so a creation-time tag duplicates data AWS already provides.

**Conclusion:** tag AWS resources with `ManagedBy=isolarium` and `Name=<--name>` only. Applied via a `default_tags` block on the AWS provider so every resource picks up `ManagedBy` without per-resource repetition, with `Name` set on the instance.

### Q31. Claude Code token expiry — correction to Q6 and a stated assumption

**Raised by user:** *"What about Claude Code tokens that expire?"* — the concern from the original idea text ("There needs to be a mechanism to refresh the Claude Code interactive session token in the EC2 instance for longer running sessions"), which Q6 deferred.

**Empirical finding.** `KeychainReader.ReadCredentials` (`internal/claude/credentials.go:16`) copies the entire Keychain item verbatim. Inspecting the structure of that item on this machine (key paths only, no values) gives:

```
claudeAiOauth.accessToken
claudeAiOauth.refreshToken
claudeAiOauth.expiresAt
claudeAiOauth.refreshTokenExpiresAt
claudeAiOauth.scopes.{0..4}
claudeAiOauth.subscriptionType
claudeAiOauth.rateLimitTier
```

**Correction to Q6.** Q6 recorded "No on-instance daemon, no refresh-token storage on EC2 — minimal new attack surface." The second clause is **false**: the blob contains `refreshToken`, so `CopyCredentials` places a refresh token on the instance. This is already true of the Lima, Docker, and nono backends today — it is not new to EC2, but EC2 raises the stakes, because the credential sits on a public-internet-reachable host that may run for weeks rather than on a local VM. A refresh token is a long-lived credential to the user's Claude subscription.

**Unverified inference, explicitly assumed.** It was initially asserted that Claude Code on the instance uses `refreshToken` to refresh its own access token and rewrites `~/.claude/.credentials.json`. This was an inference from the presence of those fields, **not a verified fact**, and it is load-bearing: if false, Q6's limitation is real, sessions die within hours, and the refresh mechanism from the original idea text returns as a v1 requirement.

**User decision:** *"Let's assume that it works as expected."* — proceed on the assumption that Claude Code on Linux refreshes its access token from `refreshToken` and rewrites the credentials file.

**Verification procedure to run before or during the spec phase** (backend-independent — the existing `vm` backend answers it, no EC2 needed, minutes rather than hours):
1. In a Lima VM with working credentials, edit `~/.claude/.credentials.json` to set `claudeAiOauth.expiresAt` to a past timestamp, leaving `refreshToken` intact.
2. Run `claude` and issue one prompt.
3. Check whether `accessToken` changed and `expiresAt` moved into the future.

If the token rotates, the assumption holds. If it errors or demands re-login, the design needs a real refresh mechanism — a host-side push of fresh credentials into the running session, or a small on-instance agent — both materially more work than anything decided so far.

**Consequence of the assumption:** the binding constraint is `refreshTokenExpiresAt` (weeks), not `expiresAt` (hours), so a long tmux session survives access-token expiry unaided. The remaining design question is therefore not *how to refresh* but **whether `isolarium run` should overwrite credentials the instance may have just refreshed.**

### Q32. Credential write policy on `isolarium run`

Given Q31's assumption (Claude Code on the instance refreshes itself), the question is not *how to refresh* but **whether `run` should overwrite credentials the instance may have just refreshed.**

**Pivotal unknown:** whether refresh tokens rotate on use. `refreshTokenExpiresAt` proves they expire; it does not prove rotation.
- *Non-rotating:* host and instance hold the same durable refresh token; overwriting is harmless — worst case pushes a stale access token and the instance refreshes again.
- *Rotating:* each refresh invalidates the previous token, so host and instance can invalidate each other regardless of write policy. A conservative policy does not fix this, but shrinks the collision window to only the moments isolarium writes.

**Clarification raised by user:** *"What do you mean by newer?"*

**Definition given:** compare `claudeAiOauth.expiresAt` — an epoch-millis field *inside the blob*, issued by the auth server and travelling with the credential. Comparing the host's value to the instance's compares two server-issued values, **not two machine clocks**, so there is no laptop-vs-EC2 skew problem. It is a *proxy*: assuming a fixed access-token TTL, a later `expiresAt` means that side refreshed more recently. It holds in both cases that matter — instance refreshed at T+5h has the later value (skip, its lineage is live); host refreshed later has the later value (write, since under rotation the instance's lineage is already dead).

| | Host `expiresAt` later | Only if unusable | Every run | API key |
|---|---|---|---|---|
| Clobbers mid-session refresh | no | no | yes | n/a |
| Repairs instance whose lineage host rotated out | yes | no — fails at next refresh | yes | n/a |
| Assumes fixed token TTL | yes | no | no | no |
| Extra work per run | one SSH read | one SSH read | none | none |
| Diverges from Lima codepath | yes | yes | no | separate auth model |

"Strip the refresh token" was dropped from consideration: under Q31's assumption it defeats primary goal #1 by killing sessions within hours.

**Answer:** **Write only when the host's `expiresAt` is later than the instance's.**

**Implications:**
- `CopyCredentials` for EC2 becomes conditional, diverging from Lima's unconditional write. Logic: read `~/.claude/.credentials.json` from the instance and parse `claudeAiOauth.expiresAt`; write the host blob if the file is absent or unparseable, or if the host's `expiresAt` is strictly greater; otherwise leave the instance alone.
- Protects a long-running tmux session that refreshed mid-flight — the central credential risk to primary goal #1.
- Still repairs an instance whose refresh lineage the host rotated out from under it, which the simpler "only if unusable" rule would not.
- Costs one SSH round-trip per `run` (~100–300ms).
- Accepted dependency: the ordering is meaningful only if access tokens have a fixed TTL. If that proves false, fall back to the "only if unusable" rule.
- File is written `0600`, as in the Lima flow.

**Security mitigations that apply regardless, to carry into the spec:**
- Credentials file at `0600`.
- **EBS root-volume encryption enabled** — the refresh token is at rest on the instance.
- `destroy` terminates the instance so the root volume is deleted (`DeleteOnTermination`).
- The /32 SG ingress (Q21) is the primary control limiting who can reach the host at all.
- README should state plainly that a Claude refresh token is placed on the EC2 instance, since this is a longer-lived exposure than the local backends.

