# EC2 Isolation Type — Implementation Plan

## Idea Type

**Type C: Platform / infrastructure capability.** This adds a fourth `Backend` implementation (`EC2Backend`) alongside `LimaBackend`, `DockerBackend`, and `NonoBackend` against the unchanged interface in `internal/backend/backend.go:14`. User-facing command semantics do not change. The substance of the work is AWS infrastructure — Terraform configuration, remote state, networking, credential handling, SSH transport.

Because the risk in this work lives almost entirely at the seams with AWS, the steel threads are cut as **journeys, not components**. Steel Thread 1 is a walking skeleton: it drives a real instance from `create` through an SSH command to `destroy`, crossing every seam at once. Each later thread thickens that working path with one scenario and is accepted by observing the result **on a real instance**, not by asserting against a fake.

The scenario table in spec section 6.2 is a coverage checklist for this plan, not its decomposition. Several of its entries are properties of the system rather than journeys through it, and those are verified as tasks inside the thread that owns the behavior.

## Instructions for Coding Agent

- IMPORTANT: Use simple commands that you have permission to execute. Avoid complex commands that may fail due to permission issues.

### Required Skills

Use these skills by invoking them before the relevant action:

| Skill | When to Use |
|-------|-------------|
| `idea-to-code:plan-tracking` | ALWAYS - track task completion in the plan file |
| `idea-to-code:tdd` | When implementing code - write failing tests first |
| `idea-to-code:commit-guidelines` | Before creating any git commit |
| `idea-to-code:incremental-development` | When writing multiple similar files (tests, classes, configs) |
| `idea-to-code:testing-scripts-and-infrastructure` | When building shell scripts or test infrastructure |
| `idea-to-code:dockerfile-guidelines` | When creating or modifying Dockerfiles |
| `idea-to-code:file-organization` | When moving, renaming, or reorganizing files |
| `idea-to-code:debugging-ci-failures` | When investigating CI build failures |
| `idea-to-code:test-runner-java-gradle` | When running tests in Java/Gradle projects |

### TDD Requirements

- NEVER write production code (`src/main/java/**/*.java`) without first writing a failing test
- Before using Write on any `.java` file in `src/main/`, ask: "Do I have a failing test?" If not, write the test first
- When task direction changes mid-implementation, return to TDD PLANNING state and write a test first

### Verification Requirements

- Hard rule: NEVER git commit, git push, or open a PR unless you have successfully run the project's test command and it exits 0
- Hard rule: If running tests is blocked for any reason (including permissions), ALWAYS STOP immediately. Print the failing command, the exact error output, and the permission/path required
- Before committing, ALWAYS print a Verification section containing the exact test command (NOT an ad-hoc command - it must be a proper test command such as `./test-scripts/*.sh`, `./scripts/test.sh`, or `./gradlew build`/`./gradlew check`), its exit code, and the last 20 lines of output

> **Language note for this Go project:** the TDD rule above applies to `internal/**/*.go` and `cmd/**/*.go` files that are not `*_test.go`. The canonical test command is `./test-scripts/test-unit.sh` (which runs `go test ./...`); the canonical build command is `make build`. `./gradlew` does not exist in this repository — ignore the Gradle rows above.

---

## Context: What Already Exists

Read these before starting. Do **not** recreate them.

| Existing artifact | State |
|---|---|
| `Makefile` | `build`, `test`, `test-integration`, `test-integration-docker`, `clean` targets |
| `.github/workflows/ci.yml` | Working CI: lint (shellcheck, gitleaks, go vet, golangci-lint), then `./test-scripts/test-end-to-end.sh --skip-docker-integration`, plus docker/e2e jobs |
| `test-scripts/test-end-to-end.sh` | Runs `clean.sh`, `test-unit.sh`, and each behavior script in sequence |
| `internal/backend/backend.go:14` | `Backend` interface — seven methods, unchanged by this work |
| `internal/backend/resolve.go:16` | `ResolveBackend` switch over `vm`, `container`, `nono` |
| `internal/backend/resolve_env.go:11` | `knownEnvironmentTypes` slice |
| `internal/backend/docker_backend.go:16` | `ExecFunc`, `ShellFunc`, `CopyCredentialsFunc` types and the injectable-function-field pattern to copy |
| `internal/cli/environment_type.go:13` | `environmentType.Set` accepting `vm`, `container`, `nono` |
| `internal/cli/cmd_create.go:11` | `defaultContainerName`, `defaultNonoName`, `resolveDefaultName` |
| `internal/cli/vm_setup.go:31` | `createAndSetupVM` — the flow to mirror |
| `internal/cli/cmd_destroy.go:35` | `destroyVM` — the "nothing to destroy, exit 0" pattern |
| `internal/cli/cmd_run.go:31` | `loadRunEnvVarsImpl` switch over isolation types |
| `internal/cli/root.go:79,105` | Two `--type` flag descriptions to update |
| `internal/status/environment.go:41` | `knownTypes` and `populateTypeSpecificFields` |
| `internal/cli/cmd_status.go:49` | `formatDetails` switch |
| `internal/config/pidconfig.go:41` | `PidConfig` struct and `validateConfig` sections |
| `internal/command/fake_runner.go` | `NewFakeRunner(t)` — use for all command-shelling unit tests |
| `internal/lima/template.yaml` | Source of the toolchain content to duplicate into cloud-init. **Do not modify it.** |
| `internal/hostscript/run.go`, `internal/envscript/run.go` | Reused unchanged by the EC2 backend |
| `design-pattern-catalog/` | Does not exist in this repo — skip the catalog lookup step of `apply-design-patterns` |

## Test Categories

Three categories, selected by Go build tag. A tagged file is invisible to `go test ./...` and only
compiles under its own tag, so nothing here can run in CI by accident or spend money unasked.

| Category | Tag | Location | Runs against | Entry point |
|---|---|---|---|---|
| Unit | *(none)* | `internal/**`, `cmd/**` | fakes and `t.TempDir()` | `./test-scripts/test-unit.sh` |
| Real-AWS lifecycle | `//go:build ec2` | `internal/ec2/*_ec2_test.go` | a real AWS account | `./test-scripts/test-ec2.sh` |
| CLI end-to-end | `//go:build e2e_ec2` | `cmd/isolarium/e2e_claude_ec2_test.go` | a real AWS account, via `bin/isolarium` | `./test-scripts/test-ec2-e2e.sh` |

`ec2` is deliberately a separate tag from the existing `integration` tag used by `internal/lima` and
`internal/docker`, so that `go test -tags=integration ./...` can never launch a billable instance.
`e2e_ec2` mirrors the existing `e2e_claude` / `e2e_gradlew` / `e2e_pytest` convention in `cmd/isolarium`.

Both AWS categories are additionally gated at runtime on `ISOLARIUM_EC2_INTEGRATION=1` and on AWS
credentials. Per the CLAUDE.md test-integrity rule, a missing gate variable or missing credentials
**fails** the test — it never skips — and the runner scripts exit non-zero when `go test` reports
`no tests to run`. `.github/workflows/ci.yml` is left unchanged and never needs AWS credentials; CI
only compiles the tagged packages (`go build -tags=ec2 ./...`) so tagged tests cannot rot unnoticed.

Every real-AWS test registers a `t.Cleanup` that destroys its instance even when an assertion fails,
so a red run never leaves a billing instance behind.

## Development Order

Steel Thread 1 is the **walking skeleton**: `create` launches a real instance, `Exec` runs a command
over SSH, `destroy` terminates it. It crosses every seam the capability depends on — CLI dispatch,
the S3 remote state backend, Terraform apply, the VPC/subnet/internet-gateway/route-table/security-group
network, the key pair, SSH reachability, metadata, teardown — and proves them together, in Task 1.8,
against a real AWS account. Its instance is deliberately bare: stock Ubuntu, no `user_data`, no
toolchain, no repository.

Threads 2–9 each thicken that working path with exactly one scenario, and each is accepted by
observing behavior on a real instance: the toolchain is present and runnable, the repository is at
the right branch, a process survives an abrupt disconnect, a second session leaves the first
untouched, `claude` authenticates and refreshes its own token, `pid.yaml` scripts leave their marker
files, `status` reports a running instance and then its absence, and `ec2 wipe` removes the shared
infrastructure from the account while retaining the state bucket.

Threads 10 and 11 harden the working paths — input and host validation, then recovery from a changed
public DNS and an undetectable host IP. Thread 10 needs no AWS account at all. Thread 12 is the
capstone: a real Claude workload driven through the CLI under `e2e_ec2`, plus the green-suite gate.

Two consequences worth stating, because they are deliberate:

- **Thread 1 is large — eight tasks.** A walking skeleton cannot be smaller than the set of seams it
  has to cross. Tasks 1.1–1.7 are `INFRA` and unit-verified; Task 1.8 is the `OUTCOME` that makes the
  thread real. Splitting them into separate threads is what produced the earlier component plan.
- **Ordering is by risk, not by the spec's scenario numbering.** The seams most likely to be wrong —
  Terraform against the S3 backend, and SSH reachability through the network configuration — are
  crossed first, when the cost of being wrong is a redesign rather than a rewrite.

Spec assumption **A1** (Claude Code on Linux refreshes its own access token from `refreshToken`) is
load-bearing. Rather than the indirect Lima-based procedure in spec 7.2, it is verified directly on
EC2 by Task 6.2, which rewinds `claudeAiOauth.expiresAt` on a real instance and asserts the token
refreshed.

## New Package Layout

All new EC2 code lives in `internal/ec2/`, mirroring `internal/lima/` and `internal/docker/`:

```
internal/ec2/
  dirs.go            paths.go helpers: EC2Dir(), TerraformDir(), PrivateKeyPath(), KnownHostsPath()
  naming.go          ValidateName
  preflight.go       CheckTerraformVersion, RequireRegion
  bucket.go          EnsureStateBucket (AWS SDK)
  scaffold.go        ExtractScaffolding (go:embed)
  terraform/         embedded stable .tf files
  keypair.go         EnsureKeypair
  publicip.go        DetectPublicIP, PersistIngressCIDR, ReadPersistedIngressCIDR
  cloud-init.yaml    embedded provisioning document
  userdata.go        RenderUserData (with 16 KB check)
  instancefile.go    WriteInstanceFile, RemoveInstanceFile, ListInstanceNames
  terraform.go       TerraformRunner: Init, Apply, OutputJSON, Destroy
  metadata.go        Metadata struct, MetadataStore
  ssh.go             BuildSSHArgs, BuildExecCommand, BuildInteractiveExecCommand
  exec.go            ExecCommand, ExecInteractiveCommand
  shell.go           OpenShell
  tmux.go            BuildTmuxCommand, NextSessionName
  session.go         CopyClaudeCredentials (conditional)
  clone.go           CloneRepo, ConfigureGitAuthor, CopyFileToInstance
  describe.go        DescribeInstance (AWS SDK)
  knownhosts.go      EvictKnownHost
  wipe.go            Wipe
internal/backend/ec2_backend.go     EC2Backend
internal/cli/ec2_setup.go           createAndSetupEC2, destroyEC2
internal/cli/cmd_ec2.go             ec2 command group + wipe subcommand
```


---

## Steel Thread 1: A real EC2 instance is created, runs a command over SSH, and is destroyed
The walking skeleton. This thread cuts vertically through every seam the capability depends on — CLI dispatch, the S3 remote state backend, Terraform apply, the VPC/subnet/internet-gateway/route-table/security-group network, the key pair, SSH reachability, metadata, and teardown — and proves them together against a real AWS account in Task 1.8. The instance carries no `user_data` and no toolchain yet; every later thread thickens this working path. Spec scenarios 1, 2, and 8.

- [x] **Task 1.1: Existing build and unit test suite pass unchanged**
  - TaskType: INFRA
  - Entrypoint: `make build && ./test-scripts/test-unit.sh`
  - Observable: `bin/isolarium` is produced and `go test ./...` reports `ok` for every package with exit code 0
  - Evidence: `make build && ./test-scripts/test-unit.sh` exits 0; its output is the baseline recorded before any EC2 code is added`
  - Steps:
    - [x] Run `make build` and confirm `bin/isolarium` is written
    - [x] Run `./test-scripts/test-unit.sh` and confirm exit code 0
    - [x] Confirm `.github/workflows/ci.yml` already invokes `./test-scripts/test-end-to-end.sh --skip-docker-integration` — do not modify CI in this task
- [x] **Task 1.2: `isolarium create --type ec2 --name my-work` is accepted and routed to `EC2Backend`**
  - TaskType: OUTCOME
  - Entrypoint: `./bin/isolarium create --type ec2 --name my-work`
  - Observable: the command reaches `EC2Backend.Create` and fails with `not yet implemented for --type ec2` on stderr — not with an unknown-type error — proving flag parsing, `resolveDefaultName`, and `ResolveBackend` all accept `ec2`; `./bin/isolarium create --type ec2` with no `--name` reports the same message for the default name `isolarium-ec2`
  - Evidence: `./test-scripts/test-ec2-preflight.sh` builds the binary and asserts both behaviors, including that the `--type ec2` failure text is the not-yet-implemented message rather than an unknown-type message`
  - Steps:
    - [x] Add `"ec2"` to the accepted values in `internal/cli/environment_type.go:13` and update the error text to `must be "vm", "container", "nono", or "ec2"`
    - [x] Update both `--type` flag descriptions in `internal/cli/root.go:79,105` to list `ec2`
    - [x] Add `defaultEC2Name = "isolarium-ec2"` next to `defaultContainerName` in `internal/cli/cmd_create.go:11` and return it from `resolveDefaultName` for `envType == "ec2"`
    - [x] Add `"ec2"` to `knownEnvironmentTypes` in `internal/backend/resolve_env.go:11`
    - [x] Create `internal/backend/ec2_backend.go` with an `EC2Backend` struct implementing all seven `Backend` methods, every one returning a `not yet implemented for --type ec2` error for now
    - [x] Add `case "ec2": return newEC2Backend(), nil` to `ResolveBackend` in `internal/backend/resolve.go:16`, with a `newEC2Backend()` factory following the shape of `newDockerBackend()`
    - [x] Reject `--work-directory` for `ec2` in `internal/cli/cmd_create.go` alongside the existing `vm` and `nono` rejections
    - [x] Add `internal/cli/cmd_create_ec2_test.go` driving the cobra root command with `--type ec2 --name my-work` and with `--type ec2` alone
    - [x] Create `test-scripts/test-ec2-preflight.sh` running `go build -o bin/isolarium ./cmd/isolarium` plus the assertions above; make it executable and add it to `test-scripts/test-end-to-end.sh` after `test-unit.sh`
    - [x] Add an `EC2 isolation (AWS)` bullet to the Features list in `README.md`
- [x] **Task 1.3: The region is resolved and the S3 state bucket is bootstrapped idempotently**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/ec2/... -run 'TestRequireRegion|TestEnsureStateBucket'`
  - Observable: `RequireRegion` returns the value of `AWS_REGION`; `EnsureStateBucket` derives `isolarium-tfstate-<account-id>-<region>` from STS `GetCallerIdentity`, then issues `CreateBucket`, `PutBucketVersioning` (Enabled), `PutBucketEncryption` (`AES256`), `PutPublicAccessBlock` (all four flags true) in that order, and returns the bucket name; a second call against a client returning `BucketAlreadyOwnedByYou` returns the same name with a nil error and still issues the three configuration calls
  - Evidence: `TestEnsureStateBucket_CreatesAndConfigures` and `TestEnsureStateBucket_IsIdempotent` in `internal/ec2/bucket_test.go` drive it with a fake S3/STS client recording an ordered call log; the real bucket is created for the first time by Task 1.8`
  - Steps:
    - [x] Add `internal/ec2/preflight_test.go` and `internal/ec2/preflight.go` with `RequireRegion(lookupEnv func(string) (string, bool)) (string, error)` returning the region when `AWS_REGION` is set; its failure messages belong to Steel Thread 10
    - [x] Add a `LookupEnvFunc` field to `EC2Backend` defaulting to `os.LookupEnv`, and call `RequireRegion` first in `Create`
    - [x] Add `github.com/aws/aws-sdk-go-v2/config`, `.../service/sts`, and `.../service/s3` to `go.mod` with `go get`, then run `go mod tidy`
    - [x] Define narrow interfaces in `internal/ec2/bucket.go` — `callerIdentityAPI` with `GetCallerIdentity`, and `stateBucketAPI` with `CreateBucket`, `PutBucketVersioning`, `PutBucketEncryption`, `PutPublicAccessBlock`
    - [x] Write `internal/ec2/bucket_test.go` first, with fakes implementing those interfaces and an ordered call log
    - [x] Implement `StateBucketName(accountID, region string) string` and `EnsureStateBucket(ctx, sts callerIdentityAPI, s3 stateBucketAPI, region string) (string, error)`; treat `BucketAlreadyOwnedByYou` as success and return every other `CreateBucket` error
    - [x] Add `newStateBucketClients(ctx, region)` building real SDK clients from the ambient environment, used only by `newEC2Backend()`
    - [x] Add an `EnsureBucketFunc` field to `EC2Backend`, wire it to `EnsureStateBucket`, and call it after `RequireRegion`
    - [x] Add the `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_SESSION_TOKEN` / `AWS_REGION` rows and the `sts:GetCallerIdentity` / `s3:*` permission list from spec 4.4 to `README.md`
- [x] **Task 1.4: The Terraform scaffolding, the Ed25519 keypair, and the host `/32` are provisioned on the host**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/ec2/... -run 'TestExtractScaffolding|TestEnsureKeypair|TestIngress'`
  - Observable: after the first call against an empty base, `<base>/ec2/terraform/` holds `provider.tf`, `backend.tf`, `network.tf`, `security.tf`, `keypair.tf`, `ami.tf`, and `variables.tf` at mode `0644` in a `0755` directory, and a second call preserves a locally edited `provider.tf` byte for byte; `<base>/ec2/id_ed25519` exists at mode `0600` with `id_ed25519.pub` at `0644` beginning `ssh-ed25519 `, and a second call reuses both unchanged; `DetectPublicIP` against a fake returning `"203.0.113.7\n"` yields `203.0.113.7/32`, `PersistIngressCIDR` writes `ingress_cidr = "203.0.113.7/32"` to `isolarium.auto.tfvars`, and no path in `publicip.go` can return `0.0.0.0/0`
  - Evidence: `TestExtractScaffolding_WritesAllFiles`, `TestExtractScaffolding_DoesNotOverwriteExistingFiles`, `TestEnsureKeypair_GeneratesKeyWithCorrectModes`, `TestEnsureKeypair_ReusesExistingKey`, `TestIngressDetectsAndPersistsCIDR`, and `TestIngressNeverReturnsOpenCIDR`, all against `t.TempDir()` bases`
  - Steps:
    - [x] Add `internal/ec2/dirs.go` with `EC2Dir`, `TerraformDir`, `PrivateKeyPath`, `PublicKeyPath`, `KnownHostsPath`, and `TfvarsPath`, each taking the base directory so tests use `t.TempDir()`
    - [x] Write `internal/ec2/scaffold_test.go` first, then `internal/ec2/scaffold.go` with `//go:embed terraform` and `ExtractScaffolding(base string) error` skipping any file already present
    - [x] Create `internal/ec2/terraform/provider.tf` — `required_version = ">= 1.10"`, the `aws` provider pinned via `required_providers`, `region = var.region`, and `default_tags { tags = { ManagedBy = "isolarium" } }`
    - [x] Create `internal/ec2/terraform/backend.tf` — `terraform { backend "s3" { use_lockfile = true } }`, partial configuration only; bucket, key, and region come from `terraform init -backend-config=...`
    - [x] Create `internal/ec2/terraform/variables.tf` declaring `ingress_cidr`, `public_key`, and `region`
    - [x] Create `internal/ec2/terraform/network.tf` — `aws_vpc` (`10.42.0.0/16`, DNS hostnames and support enabled), `aws_subnet` (`10.42.1.0/24`, `map_public_ip_on_launch = true`), `aws_internet_gateway`, `aws_route_table` with a `0.0.0.0/0` default route, and `aws_route_table_association`
    - [x] Create `internal/ec2/terraform/security.tf` — `aws_security_group` with exactly one ingress rule (`from_port = 22`, `to_port = 22`, `protocol = "tcp"`, `cidr_blocks = [var.ingress_cidr]`) and unrestricted egress
    - [x] Create `internal/ec2/terraform/keypair.tf` — `aws_key_pair` with `public_key = var.public_key`
    - [x] Create `internal/ec2/terraform/ami.tf` — `data "aws_ssm_parameter" "ubuntu_ami"` at `/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id`
    - [x] Write `internal/ec2/keypair_test.go` first, then `internal/ec2/keypair.go` with `EnsureKeypair(base string) (publicKey string, err error)` using `crypto/ed25519` plus `golang.org/x/crypto/ssh`; add `golang.org/x/crypto` with `go get` and run `go mod tidy`
    - [x] Write `internal/ec2/publicip_test.go` first with an injected `httpGetFunc`, then `internal/ec2/publicip.go` with `DetectPublicIP` against `https://checkip.amazonaws.com` (trim, `net.ParseIP`, append `/32`), `PersistIngressCIDR`, and `ReadPersistedIngressCIDR`
    - [x] Add `ExtractScaffoldingFunc`, `EnsureKeypairFunc`, and `DetectPublicIPFunc` fields to `EC2Backend` and call them in that order after the bucket bootstrap, persisting the CIDR on success
    - [x] Note the `0600` private key and the network-switch ingress behavior in the EC2 section of `README.md`
- [x] **Task 1.5: `create` writes `instance-<name>.tf`, applies, and records `metadata.json`**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/backend/... -run TestEC2Backend_Create_LaunchesInstance`
  - Observable: `<base>/ec2/terraform/instance-my-work.tf` is written containing exactly one `resource "aws_instance" "my-work"` (AMI from `data.aws_ssm_parameter.ubuntu_ami`, `instance_type = "t3.large"`, a 50 GiB `gp3` root block device with `encrypted = true` and `delete_on_termination = true`, the shared key pair, security group, and subnet, `associate_public_ip_address = true`, and `tags = { Name = "my-work" }`) plus `output "instance_id_my-work"` and `output "public_dns_my-work"`, and **no ingress rule**; the recorded terraform invocations are `init` with the three `-backend-config` flags, then `apply -auto-approve -input=false -lock-timeout=120s` with the three `-var` flags, then `output -json`; `<base>/my-work/ec2/metadata.json` holds the instance ID, public DNS, region, and `created_at`; re-running against an existing instance file errors with `instance-my-work.tf already exists; run isolarium destroy --type ec2 --name my-work first` and issues no terraform command
  - Evidence: `TestEC2Backend_Create_LaunchesInstance` and `TestEC2Backend_Create_RefusesExistingInstanceFile` in `internal/backend/ec2_backend_test.go` drive `Create` with `command.NewFakeRunner(t)` returning canned `terraform output -json`; the apply runs against real AWS for the first time in Task 1.8`
  - Steps:
    - [x] Write `internal/ec2/instancefile_test.go` first for `RenderInstanceFile(name, userData string) string`, `WriteInstanceFile` (erroring when the file exists), `RemoveInstanceFile`, and `ListInstanceNames`; assert the rendered file contains no `ingress` block and no `cidr_blocks`
    - [x] Implement `internal/ec2/instancefile.go`. `Create` passes an empty `userData` for now — Steel Thread 2 supplies the cloud-init document through the same parameter
    - [x] Write `internal/ec2/terraform_test.go` first for a `TerraformRunner` holding a `command.Runner`, a base directory, and the backend-config values, with `Init`, `Apply(vars map[string]string)`, `Destroy(vars map[string]string)`, and `OutputJSON()`; assert every `Apply`/`Destroy` carries `-auto-approve`, `-input=false`, and `-lock-timeout=120s`
    - [x] Implement `internal/ec2/terraform.go`; run `Init` only when `<base>/ec2/terraform/.terraform/` is absent
    - [x] Write `internal/ec2/metadata_test.go` first for a `Metadata` struct (`instance_id`, `public_dns`, `region`, `owner`, `repo`, `branch`, `created_at`) and a `MetadataStore` at `<base>/<name>/ec2/metadata.json` with `Write`, `Read`, and `Cleanup`, mirroring `internal/lima/metadata.go`
    - [x] Implement `internal/ec2/metadata.go` including `ParseTerraformOutput(data []byte, name string) (instanceID, publicDNS string, err error)`
    - [x] Add a `NowFunc func() time.Time` field to `EC2Backend` so `created_at` is deterministic in tests
    - [x] Wire the sequence into `EC2Backend.Create` after the host provisioning of Task 1.4
    - [x] Note in `README.md` that `create` cold start takes several minutes and that instances bill until destroyed
- [x] **Task 1.6: `Exec` runs a command over SSH and propagates its exit code**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/ec2/... ./internal/backend/... -run TestEC2Exec`
  - Observable: the command line built for `Exec` is `ssh -i <base>/ec2/id_ed25519 -o UserKnownHostsFile=<base>/ec2/known_hosts -o StrictHostKeyChecking=accept-new -o IdentitiesOnly=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=4 ubuntu@<public_dns> -- env KEY=VALUE ... <cmd>` with **no** `-t` and **no** `tmux`; a remote command exiting 42 makes `Exec` return `42, nil`; `Exec` reads `metadata.json` for the DNS and makes no AWS SDK call and no `terraform` invocation
  - Evidence: `TestEC2ExecCommandArgs` in `internal/ec2/ssh_test.go` asserts the exact argument slice; `TestEC2Backend_Exec_PropagatesExitCode` drives `Exec` through an injected exec function returning 42 and asserts `(42, nil)` plus spy AWS and terraform clients recording zero calls`
  - Steps:
    - [x] Write `internal/ec2/ssh_test.go` first for `BuildSSHArgs(base, publicDNS string, tty bool) []string`, `BuildExecCommand(base, publicDNS, workdir string, envVars map[string]string, args []string) []string`, and `BuildInteractiveExecCommand(...)`
    - [x] Implement `internal/ec2/ssh.go`; reuse a single option-set helper so `Exec`, `ExecInteractive`, `OpenShell`, and `CopyCredentials` cannot drift, mirroring `internal/lima/ssh.go`
    - [x] Sort environment-variable keys when building the `env KEY=VALUE ...` prefix, matching `buildEnvPrefix` in `internal/lima/exec.go:11`
    - [x] Add `internal/ec2/exec.go` with `ExecCommand` and `ExecInteractiveCommand` streaming stdio and mapping `*exec.ExitError` to an exit code, mirroring `internal/lima/exec.go:47`
    - [x] Define `RemoteUser = "ubuntu"` in `internal/ec2/ec2.go`. `RemoteRepoDir` arrives with Steel Thread 3; until then `Exec` runs from the login directory
    - [x] Add `ExecFunc` and `ExecInteractiveFunc` fields to `EC2Backend` and implement `Exec` and `ExecInteractive` against them
    - [x] Add `case "ec2": envNames = cfg.EC2.Run.Env` to `loadRunEnvVarsImpl` in `internal/cli/cmd_run.go:31` and add the `EC2 IsolationTypeConfig` field with tag `yaml:"ec2"` to `internal/config/pidconfig.go:41`
    - [x] Route `ec2` through `runInContainer`'s generic backend path in `internal/cli/cmd_run.go`, building env vars with `buildRunEnvVars("ec2", ...)` and calling `execBackendCommand`
    - [x] Reject `--create` for `--type ec2` with `--create is not supported with --type ec2; run isolarium create --type ec2 first`
- [x] **Task 1.7: `destroy` terminates the instance and cleans up host state**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/cli/... ./internal/backend/... -run TestEC2Destroy`
  - Observable: `<base>/ec2/terraform/instance-my-work.tf` no longer exists; the recorded command log is `terraform apply -auto-approve -input=false -lock-timeout=120s` with the three `-var` flags, followed by `ssh-keygen -R <public_dns> -f <base>/ec2/known_hosts`; `<base>/my-work/ec2/` no longer exists; a second `destroy` prints `no EC2 environment to destroy` and exits 0 without invoking terraform
  - Evidence: `TestEC2Backend_Destroy_RemovesInstanceAndHostState` and `TestEC2Backend_Destroy_IsIdempotent` in `internal/backend/ec2_backend_test.go` assert file-system state before and after plus the ordered command log`
  - Steps:
    - [x] Write `internal/ec2/knownhosts_test.go` first for `EvictKnownHost(base, publicDNS string, runner command.Runner) error`, then implement `internal/ec2/knownhosts.go` treating a missing `known_hosts` file as success
    - [x] Implement `Destroy` on `EC2Backend`: read metadata for the public DNS, resolve the ingress CIDR, remove `instance-<name>.tf`, apply, evict the known-hosts entry, then `MetadataStore.Cleanup`
    - [x] Return early with `no EC2 environment to destroy` and a nil error when `instance-<name>.tf` is absent, mirroring `destroyVM` in `internal/cli/cmd_destroy.go:35`
    - [x] Add `destroyEC2(name string) error` to a new `internal/cli/ec2_setup.go` and route `ec2` from `newDestroyCmdWithResolver` in `internal/cli/cmd_destroy.go`
    - [x] Note in `README.md` that an interrupted `destroy` is safe to re-run, and that a stale lock is cleared with `terraform force-unlock <id>`
- [x] **Task 1.8: A real EC2 instance is created, executes a command over SSH, and is destroyed**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: against a real AWS account the script creates an instance, `Exec` of `echo hello` returns `hello` on stdout with exit code 0, `Exec` of `exit 42` returns 42, `destroy` terminates the instance, and a follow-up `DescribeInstances` reports `terminated` or `shutting-down`; the S3 state bucket, VPC, subnet, internet gateway, route table, security group, and key pair all exist in the account after `create`; without `ISOLARIUM_EC2_INTEGRATION=1` the script exits non-zero with `FAIL: ISOLARIUM_EC2_INTEGRATION=1 is required to run EC2 tests`; when `go test` output contains `no tests to run`, it exits non-zero
  - Evidence: `./test-scripts/test-ec2.sh` exits 0 against a real account with the gate variable set; run without it, it exits non-zero with that exact message — the assertion runnable in CI and locally without AWS`
  - Steps:
    - [x] Create `internal/ec2/lifecycle_ec2_test.go` behind `//go:build ec2`, covering create → `Exec` of `echo hello` → `Exec` of `exit 42` → `destroy` → `DescribeInstances` confirming termination, following the shape of `internal/nono/integration_test.go`
    - [x] Have the test call `t.Fatal` (not `t.Skip`) when `ISOLARIUM_EC2_INTEGRATION` or the AWS credential variables are absent, per the CLAUDE.md test-integrity rule
    - [x] Register `t.Cleanup` that destroys the instance even when an assertion fails, so a failed run never leaks a billing instance
    - [x] Create `test-scripts/test-ec2.sh` that fails when the gate variable is unset, runs `go test -v -tags=ec2 -timeout 30m ./internal/ec2/...`, and exits non-zero when the output contains `no tests to run`
    - [x] Add a `test-ec2` target to `Makefile` running `go test -tags=ec2 -timeout 30m ./internal/ec2/...`
    - [x] Add a `--with-ec2` flag to `test-scripts/test-end-to-end.sh` following the existing `--skip-docker-integration` pattern; leave `.github/workflows/ci.yml` unchanged so CI never needs AWS credentials
    - [x] Run `shellcheck test-scripts/test-ec2.sh` and fix any findings
    - [x] Run the entrypoint against a real AWS account and record the outcome, the wall-clock duration, and the observed cold-start time in `README.md`
## Steel Thread 2: Instances come up with the full toolchain installed
Spec 3.7. The skeleton instance from Steel Thread 1 boots a stock Ubuntu image; this thread gives it the toolchain by way of a cloud-init document embedded in the binary, and proves on a real instance that every tool is actually present and runnable. The 16 KB `user_data` guard is Steel Thread 10.

- [x] **Task 2.1: cloud-init provisioning renders the full toolchain**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/ec2/... -run TestRenderUserData`
  - Observable: `RenderUserData()` returns a document starting with `#cloud-config` that installs the full toolchain and is under 16384 bytes
  - Evidence: `TestRenderUserData_ContainsToolchainAndIsUnderLimit` in `internal/ec2/userdata_test.go` asserts the `#cloud-config` prefix, the presence of each required toolchain marker, and `len < 16384``
  - Steps:
    - [x] Write `internal/ec2/userdata_test.go` first, asserting on markers for: `git`, `curl`, `wget`, `ca-certificates`, `gnupg`, `lsb-release`, `unzip`, `zip`, `uidmap`, `dbus-user-session`, `tmux`, `kernel.apparmor_restrict_unprivileged_userns=0`, `deb.nodesource.com`, `cli.github.com`, `get.docker.com/rootless`, `loginctl enable-linger`, `get.sdkman.io`, `@anthropic-ai/claude-code`, and `astral.sh/uv`
    - [x] Create `internal/ec2/cloud-init.yaml` by duplicating and adapting `internal/lima/template.yaml` per spec 3.7: root-level steps become `packages:` and `runcmd:`; user-level steps run as the `ubuntu` user via `runuser -l ubuntu -c`; `tmux` is added. Do **not** modify `internal/lima/template.yaml`
    - [x] Add a comment at the top of `internal/ec2/cloud-init.yaml` recording that it is an accepted, tracked duplicate of `internal/lima/template.yaml`, per spec 7.3 and follow-up 8.2
    - [x] Add `internal/ec2/userdata.go` with `//go:embed cloud-init.yaml` and `RenderUserData() string`
    - [x] Pass the rendered document into the `userData` parameter of `WriteInstanceFile`, which Steel Thread 1 left empty
- [x] **Task 2.2: A freshly created instance has the whole toolchain installed and working**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: on an instance created by `isolarium create --type ec2`, `Exec` of `cloud-init status --wait` reports `status: done`, and `git --version`, `gh --version`, `node --version`, `tmux -V`, `uv --version`, `claude --version`, and `docker info` (rootless, as `ubuntu`) each exit 0; `sysctl kernel.apparmor_restrict_unprivileged_userns` reports `0`
  - Evidence: `TestEC2Instance_HasToolchain` in `internal/ec2/toolchain_ec2_test.go` behind `//go:build ec2`, run by `./test-scripts/test-ec2.sh` against a real account, asserting the exit code of each version probe`
  - Steps:
    - [x] Add `internal/ec2/toolchain_ec2_test.go` behind `//go:build ec2` creating one instance, waiting on `cloud-init status --wait`, and probing each tool
    - [x] Reuse the Steel Thread 1 `t.Cleanup` destroy helper so a failed probe still tears the instance down
    - [x] Add `WaitForCloudInit` to `internal/ec2/clone.go` with an injected `SleepFunc`, capped at 15 minutes, and call it from `Create` after the apply
    - [x] Record the measured rendered `user_data` size and the observed cloud-init duration in `README.md`, since the 16 KB limit is a live constraint on this document
## Steel Thread 3: `create` places the repository inside the instance
Spec 3.10 and 3.13 steps 12–14, acceptance criterion 1. Thickens the working instance from Steel Thread 2 with the repository checkout, and proves on a real instance that the branch, the git author, and the absence of a persisted clone token are all as specified.

- [x] **Task 3.1: `create` waits for SSH, clones the repository, and configures the git author**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/backend/... -run TestEC2Backend_Create_PlacesRepository`
  - Observable: after the apply, the recorded remote command sequence is an SSH readiness probe retried until success and capped at 5 minutes, `cloud-init status --wait` capped at 15 minutes, `git clone --branch <branch> https://x-access-token:<token>@github.com/<owner>/<repo>.git repo`, a `git config user.email '<transformed>'` and `git config user.name '<name> - i2code'` pair run inside `/home/ubuntu/repo`, and one write per existing host file of `.claude/settings.local.json` and `CLAUDE.md`; no recorded command writes the token to any file on the instance
  - Evidence: `TestEC2Backend_Create_PlacesRepository` asserts the ordered remote command log from an injected SSH exec fake, including that the token appears only inside the single `git clone` argument; `TestEC2Backend_Create_TimesOutWaitingForSSH` asserts the readiness loop gives up with `instance did not become reachable over SSH within 5m0s``
  - Steps:
    - [x] Write `internal/ec2/clone_test.go` first for `WaitForSSH`, `CloneRepo`, `ConfigureGitAuthor`, and `CopyFileToInstance`
    - [x] Implement `internal/ec2/clone.go`, reusing `git.TransformEmailForIsolation` and the `" - i2code"` suffix exactly as `configureVMGitAuthor` in `internal/cli/vm_setup.go:118` does
    - [x] Inject a `SleepFunc func(time.Duration)` into the readiness loops so tests do not actually wait
    - [x] Add `createAndSetupEC2(name string) error` to `internal/cli/ec2_setup.go` following the shape of `createAndSetupVM` in `internal/cli/vm_setup.go:31`: resolve repo info, push the branch, mint the GitHub App token, then call `EC2Backend.Create`
    - [x] Route `ec2` from `newCreateCmdWithResolver` in `internal/cli/cmd_create.go:34` to `createAndSetupEC2`, alongside the existing `vm` special case
    - [x] Populate `owner`, `repo`, and `branch` in `metadata.json` from the resolved repo info
    - [x] Define `RemoteRepoDir = "/home/ubuntu/repo"` in `internal/ec2/ec2.go` and make `Exec` run from it
    - [x] Print progress lines mirroring the Lima flow: `Creating EC2 instance...`, `Waiting for cloud-init...`, `Cloning repository...`
- [x] **Task 3.2: A real instance holds the repository at the right branch with the git author configured**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: on an instance created from this repository's checkout, `Exec` of `git rev-parse --abbrev-ref HEAD` inside `/home/ubuntu/repo` prints the branch `create` was run from, `git config user.name` there ends with ` - i2code`, `git status --porcelain` there is empty, and `grep -r x-access-token /home/ubuntu` finds nothing — the clone token is never persisted on the instance
  - Evidence: `TestEC2Instance_HasRepositoryAtBranch` and `TestEC2Instance_HasNoPersistedToken` in `internal/ec2/repo_ec2_test.go` behind `//go:build ec2`, run by `./test-scripts/test-ec2.sh``
  - Steps:
    - [x] Add `internal/ec2/repo_ec2_test.go` behind `//go:build ec2` asserting the branch, the git author, a clean tree, and the absence of the token anywhere under `/home/ubuntu`
    - [x] Assert that `.claude/settings.local.json` and `CLAUDE.md` are present on the instance when they exist on the host
    - [x] Add an **EC2 isolation (AWS)** section to `README.md` describing the flow, mirroring the existing VM isolation section
## Steel Thread 4: An interactive session runs inside tmux and survives disconnect
Spec 3.6, scenario 6, acceptance criteria 3 and 4. This is the capability's primary goal — an agent session that outlives the laptop — so it is proven by actually severing the connection to a real instance and reattaching, not by inspecting a command line.

- [x] **Task 4.1: `run -i` and `shell` wrap the command in `tmux new-session -A -s isolarium`**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/ec2/... ./internal/backend/... -run TestEC2Tmux`
  - Observable: the `ExecInteractive` command line contains `-t` and ends with `tmux new-session -A -s isolarium -- <cmd>`; `OpenShell` ends with `tmux new-session -A -s isolarium -- bash -il` rooted at `/home/ubuntu/repo`; when `tmux has-session -t isolarium` succeeds beforehand, exactly the line `attaching to existing session 'isolarium'; use --new-session to start a fresh one` is written to stderr and execution proceeds without prompting; when it fails, nothing is written to stderr
  - Evidence: `TestEC2TmuxCommandWrapsInteractiveCommand` and `TestEC2TmuxNoticePrintedOnlyWhenSessionExists` in `internal/ec2/tmux_test.go` assert the argument slices and capture stderr through an injected writer`
  - Steps:
    - [x] Write `internal/ec2/tmux_test.go` first for `BuildTmuxCommand(sessionName string, args []string) []string` and `SessionExists(base, publicDNS, sessionName string, run execFunc) bool`
    - [x] Implement `internal/ec2/tmux.go` with `DefaultSessionName = "isolarium"`
    - [x] Add `internal/ec2/shell.go` with `OpenShell` building an interactive SSH command for `bash -il` inside tmux, rooted at `RemoteRepoDir`
    - [x] Add an `ErrWriter io.Writer` field to `EC2Backend` defaulting to `os.Stderr` so the notice is assertable
    - [x] Wire `ExecInteractive` and `OpenShell` on `EC2Backend` through the tmux wrapper; leave `Exec` outside tmux
    - [x] Add an `ec2` branch to `newShellCmdWithResolver` in `internal/cli/cmd_shell.go` that resolves the backend and calls `OpenShell` without the container credential copy
- [x] **Task 4.2: A long-running process on a real instance survives an abrupt disconnect and is still running on reattach**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: a command started through `ExecInteractive` inside the tmux session writes to a file on the instance once per second; killing the local SSH process abruptly leaves the remote process alive, and a second `ExecInteractive` reattaches to the same session and finds the file still growing and the same PID still running; `tmux list-sessions` on the instance reports exactly one session named `isolarium` across both connections
  - Evidence: `TestEC2Session_SurvivesDisconnect` in `internal/ec2/tmux_ec2_test.go` behind `//go:build ec2`, which starts the writer, kills the SSH child process, sleeps past several write intervals, reconnects, and asserts the PID is unchanged and the file grew`
  - Steps:
    - [x] Add `internal/ec2/tmux_ec2_test.go` behind `//go:build ec2` implementing the disconnect-and-reattach sequence
    - [x] Kill the local `ssh` process with `SIGKILL` rather than closing the session cleanly, so the test reproduces a laptop sleep or network drop rather than a graceful exit
    - [x] Assert the remote PID recorded before the disconnect is identical after reattach — a reattach that silently started a second process would otherwise pass
    - [x] Document in `README.md` that agent sessions run inside tmux, survive laptop sleep, and that the nested-tmux prefix key is `Ctrl-b Ctrl-b` when the user runs tmux locally as well
## Steel Thread 5: `--new-session` starts an additional session alongside a running one
Spec scenario 7. Thickens the tmux capability with a second concurrent session, proven on a real instance where an existing agent session must survive untouched.

- [x] **Task 5.1: `--new-session` starts an additional numbered session and never kills an existing one**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/ec2/... ./internal/cli/... -run TestEC2NewSession`
  - Observable: given remote sessions `isolarium` and `isolarium-2`, `NextSessionName` returns `isolarium-3`; given only `isolarium`, it returns `isolarium-2`; given `isolarium` and `isolarium-3`, it returns `isolarium-2`; given none, it returns `isolarium`; the resulting command line contains `tmux new-session -A -s isolarium-2` and contains neither `kill-session` nor `kill-server`; `isolarium run -i --type container --new-session -- bash` exits non-zero with `--new-session is only supported with --type ec2`
  - Evidence: `TestEC2NewSessionPicksLowestFreeNumber`, `TestEC2NewSessionNeverKills`, and `TestNewSessionRejectedForNonEC2Types` — the last driving the cobra root command for `container`, `vm`, and `nono``
  - Steps:
    - [x] Extend `internal/ec2/tmux_test.go` with `NextSessionName(existing []string) string` and `ListSessions(base, publicDNS string, run execFunc) ([]string, error)` cases, including the gap case
    - [x] Implement `ListSessions` using `tmux list-sessions -F '#{session_name}'` and `NextSessionName` selecting the lowest integer at least 2 that is unused
    - [x] Add a `NewSession bool` field to `runOptions` and a `--new-session` flag to both `run` and `shell` in `internal/cli/cmd_run.go` and `internal/cli/cmd_shell.go`
    - [x] Reject `--new-session` for every non-`ec2` type with the exact message above
    - [x] Add a `SessionNameFunc` field to `EC2Backend` resolving the session name per invocation
    - [x] Document `--new-session` in the `run` flags table of `README.md`
- [x] **Task 5.2: Two concurrent sessions run side by side on a real instance without disturbing each other**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: with a long-running process already in session `isolarium`, `run -i --new-session` opens `isolarium-2`, `tmux list-sessions` on the instance reports both, and the original process is still running with an unchanged PID after the second session exits
  - Evidence: `TestEC2Session_NewSessionLeavesExistingUntouched` in `internal/ec2/tmux_ec2_test.go` behind `//go:build ec2`, asserting both session names and the unchanged PID`
  - Steps:
    - [x] Extend `internal/ec2/tmux_ec2_test.go` with the two-session case, reusing the writer process from Steel Thread 4
    - [x] Assert the PID of the first session's process is unchanged after the second session is created and closed
## Steel Thread 6: Claude credentials reach the instance and `claude` runs authenticated there
Spec 3.11, acceptance criterion 15. The conditional-copy rule is verified with fakes; the claim that matters — that an agent can actually authenticate and refresh its own token on the instance — is verified by running `claude` on a real one. That real test replaces the indirect Lima-based A1 verification the spec proposed.

- [x] **Task 6.1: `CopyCredentials` leaves a fresher instance credential file byte-for-byte unchanged**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/ec2/... -run TestCopyClaudeCredentials`
  - Observable: with the instance reporting `claudeAiOauth.expiresAt` of `2000` and the host blob carrying `1000`, no write command is issued and the recorded command log contains only the read; with the instance at `1000` and the host at `2000`, the recorded log is `mkdir -p ~/.claude`, write, `chmod 600 ~/.claude/.credentials.json`; the same write sequence occurs when the read fails with "no such file", when the body is not valid JSON, and when `claudeAiOauth.expiresAt` is missing; with equal `expiresAt` values, no write is issued
  - Evidence: `TestCopyClaudeCredentials_SkipsWhenInstanceIsNewer`, `..._WritesWhenHostIsNewer`, `..._WritesWhenInstanceFileAbsent`, `..._WritesWhenInstanceFileUnparseable`, and `..._SkipsWhenEqual` in `internal/ec2/session_test.go` drive it with an injected SSH exec function that records every command and returns canned instance-side JSON`
  - Steps:
    - [x] Write `internal/ec2/session_test.go` first
    - [x] Add `internal/ec2/session.go` with `ReadInstanceExpiresAt(base, publicDNS string, run execFunc) (int64, bool)` and `CopyClaudeCredentials(base, publicDNS, credentials string, run execFunc) error` implementing exactly the three write conditions from spec 3.11
    - [x] Compare only the two server-issued `expiresAt` values; never call `time.Now()` in this comparison
    - [x] Wire `CopyCredentials` on `EC2Backend` to it via an injectable `CopyCredentialsFunc` field
    - [x] Have the `ec2` branch of `internal/cli/cmd_run.go` call `readKeychainCredentials()` and `b.CopyCredentials` when `--copy-session` is on, matching the container flow
- [ ] **Task 6.2: `claude` runs authenticated on a real instance and refreshes its own token there**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: after `run --copy-session`, `~/.claude/.credentials.json` on the instance is mode `0600`, and `claude -p 'reply with the single word ok'` exits 0 and prints `ok`; with `claudeAiOauth.expiresAt` on the instance then rewound to a past timestamp leaving `refreshToken` intact, a second `claude -p` run still exits 0 and `accessToken` has changed with `expiresAt` moved into the future — confirming spec assumption A1 on the platform that actually matters
  - Evidence: `TestEC2Instance_ClaudeAuthenticates` and `TestEC2Instance_RefreshesExpiredToken` in `internal/ec2/session_ec2_test.go` behind `//go:build ec2`, gated additionally on host Claude credentials being present and failing loudly when they are not`
  - Steps:
    - [ ] Add `internal/ec2/session_ec2_test.go` behind `//go:build ec2` covering the copy, the authenticated run, and the expiry-rewind refresh
    - [ ] Have the test `t.Fatal` when host Claude credentials are unavailable rather than skipping, per the CLAUDE.md test-integrity rule
    - [ ] Assert the `0600` mode on the instance-side credential file
    - [ ] Record the A1 verification outcome in `README.md`. This test supersedes the separate Lima-based A1 procedure in spec 7.2 — it verifies the assumption directly on EC2
    - [ ] Add the spec 3.11 security disclosure to `README.md`: the copied blob contains `refreshToken` and `refreshTokenExpiresAt`, a long-lived credential to the user's Claude subscription, placed on a public-internet-reachable host that may run for weeks; mitigated by `0600` file mode, root-volume encryption, `delete_on_termination`, and `/32` ingress
## Steel Thread 7: `pid.yaml` `ec2` scripts run at create time
Spec 3.12, acceptance criterion 8. Thickens `create` with the project's own script hooks, proven by marker files left on a real instance and on the host.

- [ ] **Task 7.1: `pid.yaml` `ec2` sections are validated and their paths cannot escape the project root**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/config/... -run TestValidateConfig`
  - Observable: a `pid.yaml` whose `ec2.create.creation_scripts[0].path` is `../escape.sh` is rejected by `LoadPidConfig` with `ec2.create.creation_scripts[0]: path "../escape.sh" escapes project root`, and the same holds for `ec2.create.post_creation_scripts.host_scripts` and `...env_scripts`
  - Evidence: `TestValidateConfig_RejectsEscapingEC2Paths` in `internal/config/pidconfig_test.go` drives all three new sections`
  - Steps:
    - [ ] Add failing cases to `internal/config/pidconfig_test.go` for the three `ec2.*` path-validation sections
    - [ ] Add the three `ec2.*` entries to the `sections` slice in `validateConfig` in `internal/config/pidconfig.go`
- [ ] **Task 7.2: Creation, host, and env scripts declared in `pid.yaml` actually run on a real instance**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: with a `pid.yaml` declaring an `ec2` creation script that writes `/home/ubuntu/repo/creation-ran`, a host script that writes a file on the host, and an env script that writes `/home/ubuntu/repo/env-ran`, a real `create` leaves both instance-side marker files present with the expected contents, the host-side marker present, and `ISOLARIUM_NAME` and `ISOLARIUM_TYPE=ec2` recorded in each marker from the script's own environment
  - Evidence: `TestEC2Instance_RunsPidYamlScripts` in `internal/ec2/scripts_ec2_test.go` behind `//go:build ec2`, using a fixture `pid.yaml` and asserting the three marker files and the two environment variables`
  - Steps:
    - [ ] Add creation-script, host-script, and env-script execution to `EC2Backend.Create` after repository placement, calling `hostscript.RunHostScripts(..., "ec2")` and `envscript.RunEnvScripts(..., "ec2", ...)`
    - [ ] Add `internal/ec2/scripts_ec2_test.go` behind `//go:build ec2` with the fixture `pid.yaml` and the three marker assertions
    - [ ] Assert the environment variables from inside the scripts, since a script that runs with the wrong environment would otherwise pass
    - [ ] Add an `ec2` section to this repository's own `pid.yaml` mirroring the existing `vm` section, so the capability is exercised by this project
    - [ ] Add the `ec2` `pid.yaml` block to the configuration section of `README.md`
## Steel Thread 8: `isolarium status` reports EC2 environments
Spec 3.9 state mapping, scenario 16, acceptance criterion 5. The exhaustive state mapping is a table-driven unit test; that the mapping is wired to reality is proven against a real running instance and again after it is destroyed.

- [ ] **Task 8.1: `GetState` maps every AWS instance state to an isolarium state**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/status/... ./internal/backend/... -run TestEC2State`
  - Observable: `GetState` maps AWS `running`→`running`, `stopped`→`stopped`, `stopping`→`stopped`, `pending`→`pending`, `shutting-down`→`none`, `terminated`→`none`, an absent metadata file→`none`, and a failed AWS call or missing credentials→`unknown`; rows for `vm`, `container`, and `nono` environments are unaffected
  - Evidence: `TestEC2Backend_GetState_MapsAllAWSStates` (table-driven over every mapping) and `TestListAllEnvironments_IncludesEC2Rows` in `internal/status/status_test.go`, which builds a temp base directory containing both an `ec2` and a `container` environment and asserts both rows`
  - Steps:
    - [ ] Write the table-driven state-mapping test first in `internal/backend/ec2_backend_test.go`
    - [ ] Add `github.com/aws/aws-sdk-go-v2/service/ec2` to `go.mod` with `go get`, then run `go mod tidy`
    - [ ] Write `internal/ec2/describe_test.go` first for `DescribeInstance(ctx, api describeInstancesAPI, instanceID string) (publicDNS, state string, err error)` against a narrow interface with a fake, then implement `internal/ec2/describe.go`
    - [ ] Add a `DescribeInstanceFunc` field to `EC2Backend` and implement `GetState` reading `metadata.json` and calling it
    - [ ] Add `"ec2"` to `knownTypes` in `internal/status/environment.go:41`
    - [ ] Add an `ec2` case to `populateTypeSpecificFields` populating `Repository` and `Branch` from the same `owner`/`repo`/`branch` fields the `vm` case uses
    - [ ] Add `"ec2"` to the `vm` case of `formatDetails` in `internal/cli/cmd_status.go:49` so the repository-and-branch format is shared
- [ ] **Task 8.2: `isolarium status` reports a real instance as running, then as gone after destroy**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: with a real instance up, `isolarium status` emits a row `my-work  ec2  running  humansintheloop-dev/isolarium (<branch>)`; after `isolarium destroy --type ec2 --name my-work`, no `ec2` row for that name appears; with `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` unset while the instance still exists, the row shows `unknown` rather than failing the command
  - Evidence: `TestEC2Instance_StatusReportsRunningThenGone` and `TestEC2Instance_StatusDegradesWithoutCredentials` in `internal/ec2/status_ec2_test.go` behind `//go:build ec2`, driving the built binary and parsing its stdout`
  - Steps:
    - [ ] Add `internal/ec2/status_ec2_test.go` behind `//go:build ec2` covering the running row, the post-destroy absence, and the credential-less degradation
    - [ ] Run the credential-less case by clearing the AWS variables for that invocation only, so the surrounding test keeps its credentials
    - [ ] Add an `isolarium status` example row for `ec2` to `README.md`
## Steel Thread 9: `isolarium ec2 wipe` tears down shared infrastructure
Spec 3.13 `ec2 wipe`, scenarios 11 and 12, acceptance criterion 7. Teardown of the shared VPC, security group, and key pair, proven against the AWS API rather than against local files, with the state bucket deliberately retained.

- [ ] **Task 9.1: `ec2 wipe` refuses while instances exist and otherwise tears down shared infrastructure**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/cli/... ./internal/ec2/... -run TestEC2Wipe`
  - Observable: with `instance-my-work.tf` and `instance-other.tf` present, `wipe` exits non-zero, no terraform command is invoked, and stderr lists both names plus `run isolarium destroy --type ec2 --name my-work` and `... --name other`; with no instance files present, the recorded command is `terraform destroy -auto-approve -input=false -lock-timeout=120s` with the three `-var` flags, then `<base>/ec2/terraform/`, `<base>/ec2/id_ed25519`, `<base>/ec2/id_ed25519.pub`, and `<base>/ec2/known_hosts` no longer exist, and stdout contains `S3 state bucket isolarium-tfstate-<account>-<region> was intentionally retained; see the README for manual removal`
  - Evidence: `TestEC2Wipe_RefusesWhenInstancesExist` and `TestEC2Wipe_TearsDownAndReportsRetainedBucket` in `internal/ec2/wipe_test.go` assert the command log and the file-system state before and after, driven through the cobra `ec2 wipe` command in `internal/cli/cmd_ec2_test.go``
  - Steps:
    - [ ] Write `internal/ec2/wipe_test.go` first
    - [ ] Implement `internal/ec2/wipe.go` with `Wipe(base string, deps WipeDeps) error` enumerating instance files via `ListInstanceNames` and refusing when any exist
    - [ ] Create `internal/cli/cmd_ec2.go` adding an `ec2` command group with a `wipe` subcommand, registered in `newRootCmdWithResolvers` in `internal/cli/root.go`
    - [ ] Add an `ec2 wipe` row to the Commands table of `README.md`, plus manual state-bucket removal instructions (delete every object version, then delete the bucket)
- [ ] **Task 9.2: `ec2 wipe` removes the shared AWS infrastructure from a real account and retains the bucket**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: with one instance up, `isolarium ec2 wipe` exits non-zero and the VPC still exists; after destroying that instance, `wipe` exits 0 and `DescribeVpcs`, `DescribeSecurityGroups`, and `DescribeKeyPairs` filtered on the `ManagedBy = isolarium` tag return nothing, while `HeadBucket` on the state bucket still succeeds
  - Evidence: `TestEC2Wipe_RefusesWithLiveInstance` and `TestEC2Wipe_RemovesInfraAndRetainsBucket` in `internal/ec2/wipe_ec2_test.go` behind `//go:build ec2`, asserting against the AWS API rather than against the local file system`
  - Steps:
    - [ ] Add `internal/ec2/wipe_ec2_test.go` behind `//go:build ec2` covering the refusal case and the successful teardown
    - [ ] Assert the absence of the VPC, security group, and key pair through the AWS API, since local file removal proves nothing about the account
    - [ ] Assert `HeadBucket` still succeeds, since retaining the state bucket is the specified behavior
    - [ ] Run this test last in the `ec2` suite, as it removes the shared infrastructure every other `ec2` test depends on
## Steel Thread 10: `create` rejects invalid input and misconfigured hosts before any AWS call
Spec scenarios 13, 14, 15, and 17; acceptance criterion 11. Guards on a path that already works. Every check here runs before the S3 bootstrap or before Terraform is invoked, so no AWS resource can be created by a doomed run — and none of these tests needs an AWS account.

- [ ] **Task 10.1: `isolarium create --type ec2 --name My_Env` is rejected with the naming rule**
  - TaskType: OUTCOME
  - Entrypoint: `./bin/isolarium create --type ec2 --name My_Env`
  - Observable: exit code is non-zero and stderr contains `invalid --name "My_Env" for --type ec2: must match ^[a-z][a-z0-9-]{0,31}$`; `./bin/isolarium create --type ec2 --name my-work` still gets past name validation
  - Evidence: `./test-scripts/test-ec2-preflight.sh` gains a case that runs the entrypoint, asserts non-zero exit and the message text, then runs the same command with `--name my-work` and asserts the failure message is *not* the naming message`
  - Steps:
    - [ ] Add `internal/ec2/naming_test.go` covering: accepts `a`, `my-work`, a 32-char name; rejects `My_Env`, `1abc`, `-abc`, empty, a 33-char name
    - [ ] Add `internal/ec2/naming.go` with `ValidateName(name string) error` enforcing `^[a-z][a-z0-9-]{0,31}$`
    - [ ] Call `ec2.ValidateName` as the first statement of `EC2Backend.Create`, before `RequireRegion`
    - [ ] Add the assertion case to `test-scripts/test-ec2-preflight.sh`
- [ ] **Task 10.2: `create --type ec2` fails fast when `AWS_REGION` is unset**
  - TaskType: OUTCOME
  - Entrypoint: `env -u AWS_REGION ./bin/isolarium create --type ec2 --name my-work`
  - Observable: exit code non-zero, stderr contains `AWS_REGION is required for --type ec2; set it in .env.local`, and no directory `~/.isolarium/ec2/` is created by the run
  - Evidence: `./test-scripts/test-ec2-preflight.sh` gains a case that runs the entrypoint with `HOME` pointed at a fresh temp directory, asserts the message and non-zero exit, and asserts `$HOME/.isolarium/ec2` does not exist afterwards`
  - Steps:
    - [ ] Add cases to `internal/ec2/preflight_test.go` for `RequireRegion` returning an error naming `AWS_REGION` when the variable is unset or empty
    - [ ] Extend `RequireRegion` in `internal/ec2/preflight.go` with those error paths
    - [ ] Confirm with a spy `EnsureBucketFunc` that no AWS call precedes the check
    - [ ] Add the assertion case to `test-scripts/test-ec2-preflight.sh`
- [ ] **Task 10.3: `create --type ec2` fails with an actionable message when `terraform` is below 1.10**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/backend/... -run TestEC2Backend_Create`
  - Observable: `EC2Backend.Create` returns an error containing `terraform 1.10.0 or later is required for --type ec2 (found 1.9.8)` when the injected terraform runner reports `1.9.8`, and gets past the version gate when it reports `1.10.5`; the state-bucket function is never invoked in the failing case
  - Evidence: `TestEC2Backend_Create_RejectsOldTerraform` and `TestEC2Backend_Create_AcceptsTerraform110` in `internal/backend/ec2_backend_test.go` invoke `Create` with a `command.FakeRunner` returning canned `terraform version -json` output and a spy bucket function`
  - Steps:
    - [ ] Add `internal/ec2/preflight_test.go` cases for `CheckTerraformVersion(runner command.Runner) error` — parses `.terraform_version` from `terraform version -json`, accepts `1.10.0`, `1.10.5`, `1.12.1`, rejects `1.9.8` and `0.15.0`, and errors clearly when the JSON is unparseable or the binary is missing
    - [ ] Implement `CheckTerraformVersion` using a numeric major/minor/patch comparison, not string comparison
    - [ ] Call it from `EC2Backend.Create` after `RequireRegion` and before the state-bucket bootstrap
    - [ ] Add the `terraform` >= 1.10 row to the Prerequisites table in `README.md`
- [ ] **Task 10.4: `create` fails before apply when rendered `user_data` exceeds 16 KB**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/ec2/... -run TestValidateUserDataSize`
  - Observable: `ValidateUserDataSize` on a 16385-byte document returns an error containing `rendered user_data is 16385 bytes, exceeding the EC2 limit of 16384 bytes`, and returns nil at exactly 16384
  - Evidence: `TestValidateUserDataSize_RejectsOversizeDocument` and `TestValidateUserDataSize_AcceptsExactLimit` drive the boundary in `internal/ec2/userdata_test.go``
  - Steps:
    - [ ] Add the boundary cases to `internal/ec2/userdata_test.go` first
    - [ ] Add `ValidateUserDataSize(doc string) error` to `internal/ec2/userdata.go`
    - [ ] Call it from `EC2Backend.Create` immediately after `RenderUserData`, so the failure precedes any Terraform invocation
## Steel Thread 11: `run` and `destroy` recover from environment changes
Spec 3.9 refresh-on-failure and the spec 3.4 destroy-time ingress fallback; scenario 5; acceptance criterion 14. Recovery behaviors layered onto paths that already work — the DNS refresh is proven by actually stopping and starting a real instance.

- [ ] **Task 11.1: An SSH connect failure triggers exactly one `DescribeInstances` refresh and one retry**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/backend/... -run TestEC2Backend_Exec_RefreshesMetadata`
  - Observable: when the first SSH attempt fails with a connect error, `DescribeInstances` is called exactly once with the cached instance ID, `metadata.json` is rewritten with the new `public_dns` (instance ID unchanged), and SSH is attempted exactly once more against the new DNS; when the remote command merely exits non-zero, no refresh occurs and the exit code is returned as-is; when the retry also fails to connect, the error is returned after exactly two SSH attempts and one refresh — never more
  - Evidence: `TestEC2Backend_Exec_RefreshesMetadataOnConnectFailure`, `TestEC2Backend_Exec_DoesNotRefreshOnNonZeroExit`, and `TestEC2Backend_Exec_RetriesAtMostOnce` in `internal/backend/ec2_backend_test.go` count calls on injected SSH and describe fakes and assert the rewritten metadata file`
  - Steps:
    - [ ] Add a sentinel `ErrSSHConnect` in `internal/ec2/exec.go`, returned when `ssh` exits with code 255 or fails to start, distinguishing connect failure from remote non-zero exit
    - [ ] Add the single-retry wrapper shared by `Exec`, `ExecInteractive`, and `OpenShell`, using the `DescribeInstanceFunc` added in Steel Thread 8
    - [ ] Document the refresh-on-failure behavior in the EC2 section of `README.md`
- [ ] **Task 11.2: `run` recovers on a real instance whose public DNS changed after a stop and start**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2.sh`
  - Observable: after stopping and starting a real instance out of band through the AWS API, its public DNS differs from the one in `metadata.json`; the next `run -- echo hello` still prints `hello` and exits 0, and `metadata.json` afterwards holds the new DNS with the instance ID unchanged
  - Evidence: `TestEC2Instance_RecoversFromChangedDNS` in `internal/ec2/recovery_ec2_test.go` behind `//go:build ec2`, which stops and starts the instance through the SDK, asserts the DNS actually changed before running the command, and fails the test if it did not`
  - Steps:
    - [ ] Add `internal/ec2/recovery_ec2_test.go` behind `//go:build ec2` implementing the stop/start/reconnect sequence
    - [ ] Assert that the DNS genuinely changed before exercising the recovery, so the test cannot pass vacuously when AWS happens to reassign the same name
    - [ ] Allow generous timeouts — a stop and start cycle takes minutes — and register cleanup that destroys the instance regardless of outcome
- [ ] **Task 11.3: Public-IP detection failure aborts `create` but falls back to the persisted CIDR on `destroy` and `wipe`**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/ec2/... ./internal/backend/... -run TestResolveIngressCIDR`
  - Observable: `ResolveIngressCIDR(base, opCreate)` returns the detection error unchanged and no instance is launched; `ResolveIngressCIDR(base, opDestroy)` falls back to the value persisted in `isolarium.auto.tfvars` and returns it with a warning string, and errors when no persisted value exists; with detection failing and the file holding `ingress_cidr = "198.51.100.4/32"`, `destroy` writes the warning to stderr, still runs the apply with that CIDR, and completes; no failure path returns `0.0.0.0/0`
  - Evidence: `TestResolveIngressCIDR_FailureIsFatalOnCreate`, `TestResolveIngressCIDR_FallsBackOnDestroy`, and `TestEC2Backend_Destroy_FallsBackToPersistedCIDR` in `internal/ec2/publicip_test.go` and `internal/backend/ec2_backend_test.go``
  - Steps:
    - [ ] Add the failure-policy cases to `internal/ec2/publicip_test.go` first
    - [ ] Add an `operation` enum with `opCreate` and `opDestroy`, and `ResolveIngressCIDR(base string, op operation, get httpGetFunc) (cidr string, warning string, err error)` implementing the per-operation policy from spec 3.4
    - [ ] Route `EC2Backend.Create` through `opCreate`, and both `EC2Backend.Destroy` and `ec2.Wipe` through `opDestroy`, writing any warning to `ErrWriter`
    - [ ] Document the per-operation policy in the EC2 section of `README.md`
## Steel Thread 12: An agent workload runs end to end and the full suite is green
Spec 5.5 and acceptance criteria 16–19; CLAUDE.md test-integrity rule. The capstone: a real Claude workload driven through the CLI in an EC2 environment under `//go:build e2e_ec2`, mirroring the existing `e2e_claude` tests, plus the final green-suite gate.

- [ ] **Task 12.1: An agent workload runs to completion in an EC2 environment through the CLI**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-ec2-e2e.sh`
  - Observable: driving the built `bin/isolarium` binary end to end — `create --type ec2`, `run -i --type ec2` with a Claude prompt that edits a file in the repository and commits it, `run --type ec2 -- git log -1 --format=%s` confirming the commit message, then `destroy --type ec2` — exits 0 with the instance terminated; without `ISOLARIUM_EC2_INTEGRATION=1` the script exits non-zero with `FAIL: ISOLARIUM_EC2_INTEGRATION=1 is required to run EC2 end-to-end tests`
  - Evidence: `./test-scripts/test-ec2-e2e.sh` exits 0 against a real account with the gate set, and exits non-zero with that exact message without it`
  - Steps:
    - [ ] Add `cmd/isolarium/e2e_claude_ec2_test.go` behind `//go:build e2e_ec2`, following the shape of the existing `e2e_claude_vm_test.go`
    - [ ] Reuse the helpers in `cmd/isolarium/e2e_claude_helpers_test.go`; add `e2e_ec2` to the shared build constraint on that file so the helpers compile under the new tag
    - [ ] Create `test-scripts/test-ec2-e2e.sh` running `go test -v -tags=e2e_ec2 -timeout 45m ./cmd/isolarium/...`, failing when the gate variable is unset and when the output contains `no tests to run`
    - [ ] Add a `test-e2e-ec2` target to `Makefile`
    - [ ] Run `shellcheck test-scripts/test-ec2-e2e.sh` and fix any findings
- [ ] **Task 12.2: The full test suite and build pass with the EC2 backend present**
  - TaskType: INFRA
  - Entrypoint: `make build && ./test-scripts/test-end-to-end.sh --skip-docker-integration`
  - Observable: the build produces `bin/isolarium`, every unit test package reports `ok`, `test-ec2-preflight.sh` passes, and the suite prints `=== All tests passed ===` with exit code 0; `go build -tags=ec2 ./...` and `go build -tags=e2e_ec2 ./...` both succeed, so the tagged tests cannot rot unnoticed
  - Evidence: `make build && ./test-scripts/test-end-to-end.sh --skip-docker-integration` exits 0; this is the same command `.github/workflows/ci.yml` runs, so a green local run predicts a green CI run`
  - Steps:
    - [ ] Run `go vet ./...` and fix any findings
    - [ ] Run `golangci-lint run` and fix any findings, since CI gates on it
    - [ ] Add a compile-only check of both new tags to CI — `go build -tags=ec2 ./...` and `go build -tags=e2e_ec2 ./...` — so tagged tests stay compiling without CI ever needing AWS credentials
    - [ ] Run `shellcheck test-scripts/*.sh` and fix any findings
    - [ ] Run the entrypoint and confirm exit code 0
    - [ ] Run `./test-scripts/test-ec2.sh` and `./test-scripts/test-ec2-e2e.sh` against a real account one final time and record both results
    - [ ] Verify every item of spec section 9 acceptance criteria 16–19 is satisfied, and that `README.md` documents the `terraform` >= 1.10 prerequisite, required environment variables, cold-start latency, billing until destroyed, the refresh token on the instance, `terraform force-unlock` recovery, manual state-bucket removal, and the nested-tmux prefix-key caveat
## Change History
### 2026-08-19 16:39 - reorder-threads
Develop the happy path first: the create -> run -> shell -> destroy spine and its real-AWS end-to-end proof now precede the guardrail threads (preflight rejection, user_data size limit, DNS-refresh recovery) and the secondary capabilities (credentials, pid.yaml scripts, status, wipe).

### 2026-08-19 16:40 - replace-task
Happy path first: prove that --type ec2 is accepted and dispatched to EC2Backend rather than proving that an invalid name is rejected. Name validation moves to the input-and-host-validation thread.

### 2026-08-19 16:40 - replace-task
The happy path needs the region resolved before the bucket can be named, so RequireRegion's success behavior moves here from the preflight thread that now runs later. Only its fail-fast error paths stay deferred.

### 2026-08-19 16:40 - replace-thread
Happy path first: this thread now proves that detection, the /32 conversion, and persistence work. The ResolveIngressCIDR failure policy moves to the recovery thread.

### 2026-08-19 16:40 - replace-thread
Happy path first: this thread now produces the provisioning document the instance actually needs. ValidateUserDataSize, which is a guard rather than a capability, moves to the validation thread.

### 2026-08-19 16:41 - replace-task
The user_data size check now lands in a later validation thread, so the wiring step sequences Create after RenderUserData instead.

### 2026-08-19 16:41 - replace-task
Destroy now proves the happy teardown path only. The persisted-CIDR fallback, which depends on the deferred ResolveIngressCIDR failure policy, moves to the recovery thread.

### 2026-08-19 16:41 - replace-thread
Split the old end-to-end thread: the real-AWS lifecycle proof belongs immediately after destroy so the happy path is validated before hardening; the full-suite-green gate becomes the plan's final thread.

### 2026-08-19 16:42 - replace-thread
Collects the guards deferred out of the happy-path threads — name validation from thread 1, the user_data 16 KB check from thread 5 — alongside the existing region and terraform-version preflight checks.

### 2026-08-19 16:42 - replace-thread
Collects the recovery behaviors deferred out of the happy-path threads: the ResolveIngressCIDR failure policy from thread 4 and the destroy-time persisted-CIDR fallback from thread 10 join the DNS refresh.

### 2026-08-19 16:42 - insert-thread-after
Restores the full-suite gate that was split out of the old end-to-end thread, now as the plan's final thread.

### 2026-08-19 16:44 - replace-task
Removes a forward reference to the opDestroy policy, which now lands in Steel Thread 17 after wipe is built.

### 2026-08-19 16:44 - replace-task
Routes wipe through the opDestroy policy too, closing the forward reference removed from the wipe thread.

### 2026-08-19 16:56 - delete-thread
Re-cutting the plan into genuine steel threads; this thread's content is redistributed into threads whose acceptance is real behavior on a real instance.

### 2026-08-19 16:56 - delete-thread
Re-cutting the plan into genuine steel threads; this thread's content is redistributed into threads whose acceptance is real behavior on a real instance.

### 2026-08-19 16:56 - delete-thread
Re-cutting the plan into genuine steel threads; this thread's content is redistributed into threads whose acceptance is real behavior on a real instance.

### 2026-08-19 16:56 - delete-thread
Re-cutting the plan into genuine steel threads; this thread's content is redistributed into threads whose acceptance is real behavior on a real instance.

### 2026-08-19 16:56 - delete-thread
Re-cutting the plan into genuine steel threads; this thread's content is redistributed into threads whose acceptance is real behavior on a real instance.

### 2026-08-19 16:56 - delete-thread
Re-cutting the plan into genuine steel threads; this thread's content is redistributed into threads whose acceptance is real behavior on a real instance.

### 2026-08-19 16:58 - replace-thread
Re-cut as a genuine steel thread: the old threads 1-7 and 10 built the bucket, scaffolding, keypair, ingress, instance file, SSH transport, and destroy as separate mock-verified components, deferring every integration risk to a single end-of-plan test. They are now the tasks of one thread whose acceptance is a real instance answering a real SSH command.

### 2026-08-19 16:58 - delete-thread
Absorbed into Steel Thread 1, the walking skeleton, where this component is now a task verified by a real instance rather than a mock.

### 2026-08-19 16:58 - delete-thread
Absorbed into Steel Thread 1, the walking skeleton, where this component is now a task verified by a real instance rather than a mock.

### 2026-08-19 16:58 - delete-thread
Absorbed into Steel Thread 1, the walking skeleton, where this component is now a task verified by a real instance rather than a mock.

### 2026-08-19 16:58 - delete-thread
Absorbed into Steel Thread 1, the walking skeleton, where this component is now a task verified by a real instance rather than a mock.

### 2026-08-19 16:58 - delete-thread
Absorbed into Steel Thread 1, the walking skeleton, where this component is now a task verified by a real instance rather than a mock.

### 2026-08-19 16:58 - delete-thread
Absorbed into Steel Thread 1, the walking skeleton, where this component is now a task verified by a real instance rather than a mock.

### 2026-08-19 16:58 - delete-thread
Absorbed into Steel Thread 1, the walking skeleton, where this component is now a task verified by a real instance rather than a mock.

### 2026-08-19 16:58 - replace-thread
The thread now ends in a real instance answering version probes rather than in a string-marker assertion against an embedded file.

### 2026-08-19 16:59 - replace-thread
Adds a real-instance acceptance task; the token-never-persisted property is now checked on the machine rather than in a recorded command log.

### 2026-08-19 17:00 - replace-thread
The survives-disconnect claim is only meaningful against a real instance; the old thread asserted argument slices and never disconnected anything. --new-session is split into its own thread.

### 2026-08-19 17:00 - insert-thread-after
Split out of the old tmux thread so each thread carries one scenario, and given a real-instance acceptance task.

### 2026-08-19 17:00 - replace-thread
Turns the load-bearing A1 assumption into a directly executed test on EC2 rather than a manual procedure run against a different backend.

### 2026-08-19 17:01 - insert-thread-after
Acceptance is now marker files on a real instance rather than an ordered log from an injected spy.

### 2026-08-19 17:01 - insert-thread-after
Adds a real-instance acceptance task so the state mapping is proven end to end rather than only against a fake DescribeInstances.

### 2026-08-19 17:02 - insert-thread-after
Acceptance now asserts through DescribeVpcs/DescribeSecurityGroups/DescribeKeyPairs; removing local files proves nothing about the account.

### 2026-08-19 17:02 - insert-thread-after
Collects every guard deferred out of the happy-path threads into one thread that runs entirely without AWS.

### 2026-08-19 17:02 - insert-thread-after
The refresh-on-failure claim is verified against a real DNS change rather than an injected connect error alone.

### 2026-08-19 17:02 - insert-thread-after
Adds the CLI-level e2e_ec2 capstone alongside the full-suite gate, and requires CI to compile both new tags so tagged tests cannot rot.

### 2026-08-19 17:28 - mark-task-complete
ec2 accepted by --type flag, resolveDefaultName, and ResolveBackend; EC2Backend stub returns 'not yet implemented for --type ec2'; verified by go tests and test-scripts/test-ec2-preflight.sh

### 2026-08-19 19:27 - mark-task-complete
Exec and ExecInteractive run over SSH via BuildSSHArgs/BuildExecCommand; exit codes propagate; run --type ec2 routed with ec2 run.env and --create rejected

### 2026-08-19 19:42 - mark-task-complete
Destroy removes instance-<name>.tf, re-applies, evicts the known_hosts entry, and cleans up host metadata; a second destroy reports 'no EC2 environment to destroy' and exits 0. destroyEC2 takes an io.Writer so the CLI can assert on the message, a small deviation from the planned destroyEC2(name string) signature.

### 2026-08-19 20:28 - mark-step-complete
Added internal/ec2/lifecycle_ec2_test.go behind //go:build ec2 covering create, Exec of echo hello, Exec of exit 42, destroy, and DescribeInstances

### 2026-08-19 20:28 - mark-step-complete
requireIntegrationGate and requireAWSCredentials call t.Fatalf, never t.Skip

### 2026-08-19 20:28 - mark-step-complete
t.Cleanup(destroyIfStillRunning) is registered before Create so a failed assertion still tears the instance down

### 2026-08-19 20:28 - mark-step-complete
test-scripts/test-ec2.sh gates on ISOLARIUM_EC2_INTEGRATION, runs the tagged tests, and fails on 'no tests to run'

### 2026-08-19 20:28 - mark-step-complete
Added the test-ec2 target to the Makefile

### 2026-08-19 20:28 - mark-step-complete
Added --with-ec2 to test-scripts/test-end-to-end.sh; .github/workflows/ci.yml is unchanged

### 2026-08-19 20:28 - mark-step-complete
shellcheck reports no findings for test-scripts/test-ec2.sh

### 2026-08-19 21:35 - mark-step-complete
Ran ./test-scripts/test-ec2.sh against the real AWS account in us-west-1; it exited 0 and the measured wall clock, create time, and cold start are recorded in README.md

### 2026-08-19 21:35 - mark-task-complete
The entrypoint creates a real instance, runs echo hello and exit 42 over SSH, destroys it, and confirms termination; it fails without ISOLARIUM_EC2_INTEGRATION=1

### 2026-08-20 08:02 - mark-task-complete
Verified against a real AWS account: cloud-init reports status: done in 1m37s and every toolchain probe exits 0. Required two supporting fixes discovered by the run - a generated aws_key_pair name with create_before_destroy so a rotated host keypair cannot leave a new instance holding the superseded key, and publishing the user-level toolchain to /etc/environment plus loading nf_tables so uv and rootless docker are reachable from the non-interactive ssh that Exec uses.

### 2026-08-20 09:01 - mark-task-complete
Real-AWS run proved the branch, isolated git author, clean tree, and project config on the instance. The token-persistence assertion failed against a real instance because git clone records the authenticated URL in .git/config, so CloneRepo now rewrites origin to the credential-free URL and run injects a per-run token via GIT_CONFIG insteadOf.

### 2026-08-20 10:16 - mark-task-complete
Proved on a real EC2 instance: a writer started through ExecInteractive kept the same PID and kept growing its log after its local ssh process was SIGKILLed, and tmux list-sessions reported exactly one isolarium session across both connections.

### 2026-08-20 11:52 - mark-task-complete
TestEC2Session_NewSessionLeavesExistingUntouched passes on a real instance; ./test-scripts/test-ec2.sh green end to end
