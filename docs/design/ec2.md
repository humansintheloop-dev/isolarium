# EC2 isolation type

The `ec2` isolation type runs an environment on its own EC2 instance. The host
talks to the instance over SSH and manages it with Terraform. The backend is
`EC2Backend` in `internal/backend/ec2_backend.go`; the helpers it uses are in
`internal/ec2/`.

# Operations

`EC2Backend` implements every method of the `backend.Backend` interface
(`internal/backend/backend.go`). The CLI verbs map to those methods as follows.

| CLI | Backend method |
|---|---|
| `isolarium create --type ec2 --name <name>` | `Create` |
| `isolarium destroy --type ec2 --name <name>` | `Destroy` |
| `isolarium run --type ec2 --name <name> -- <cmd>` | `Exec` |
| `isolarium run -i --type ec2 --name <name> -- <cmd>` | `ExecInteractive` |
| `isolarium shell --type ec2 --name <name>` | `OpenShell` |
| `isolarium status` | `GetState` |
| `isolarium shell` and `isolarium run` (`--copy-session`, on by default) | `CopyCredentials` |
| `isolarium ec2 wipe` | not a `Backend` method; see below |

## Create

`Create(opts CreateOptions)` takes the environment from nothing to an instance
that holds the repository and has run the project's `pid.yaml` hooks. In order:

1. Validate the name (`ec2.ValidateName`). A bad name is rejected before AWS is
   touched.
2. Load `pid.yaml` from the work directory. A configuration that names an
   escaping script path is rejected here, before anything is created.
3. Resolve the AWS account: require `AWS_REGION`, check the host's Terraform
   version supports S3 native state locking, then ensure the remote-state
   bucket exists. The version check comes before the bucket so an unsupported
   host creates nothing in the account.
4. Provision host state: extract the embedded Terraform scaffolding into
   `<metadata>/ec2/terraform/` (existing files are left alone), ensure the
   Ed25519 keypair under `<metadata>/ec2/`, and detect the host's public IP to
   pin SSH ingress to it.
5. Render cloud-init user data, check it fits the user-data size limit, and
   write `instance-<name>.tf`. Create refuses if that file already exists.
6. Run `terraform init` and `terraform apply`, then read `terraform output
   -json` and pick out `instance_id_<name>` and `public_dns_<name>`.
7. Resolve the repository source. This pushes the current branch and mints a
   short-lived clone token, so it is deferred until an instance exists.
8. Write `metadata.json` (instance ID, public DNS, region, owner, repo, branch,
   created-at). This happens before waiting on the instance so a create that
   times out during provisioning can still be destroyed.
9. Wait for SSH, then wait for cloud-init to finish. A degraded cloud-init run
   is reported with the modules that failed.
10. Clone the repository onto the instance.
11. Run the `pid.yaml` `ec2.create` hooks: creation scripts on the instance,
    then host scripts on the host, then post-creation env scripts on the
    instance. A script that exits non-zero fails the create.

## Destroy

`Destroy(name)` removes one environment and leaves the rest, and the shared
infrastructure, in place.

- If `instance-<name>.tf` does not exist, it prints `no EC2 environment to
  destroy` and returns success.
- Otherwise it resolves the account (region and bucket), keypair, and ingress
  CIDR first, so a failure there leaves the environment intact. It skips the
  Terraform version check: the environment was created by a Terraform that
  passed it, and refusing to destroy would strand a running instance.
- It deletes `instance-<name>.tf` and runs `terraform apply` again rather than
  a targeted destroy. A resource that is in state but has no configuration is
  always planned for destruction, so an interrupted destroy converges when it
  is re-run.
- It evicts the instance's host key from `<metadata>/ec2/known_hosts` (if
  metadata recorded a public DNS) and removes `metadata.json`.

## Exec

`Exec(req)` runs a non-interactive command over SSH in the repository
directory on the instance and returns the remote exit code. The address comes
from `metadata.json`, so no AWS or Terraform call is made.

## ExecInteractive

`ExecInteractive(req)` runs the command inside a tmux session on the instance,
so a dropped SSH connection leaves the command running. By default it joins
the single shared session; if that session already exists, tmux discards the
command it was handed and attaches instead, and the backend announces the
reattach on stderr first. `--new-session` (`UseNewSession`) makes it join a
session no other invocation holds.

## OpenShell

`OpenShell(req)` opens an interactive shell inside the instance's tmux session,
with the same session resolution and `--new-session` behaviour as
`ExecInteractive`.

## Address refresh

`Exec`, `ExecInteractive`, and `OpenShell` all go through `onInstance`. The
instance ID is fixed but the public DNS changes across a stop/start, so if SSH
cannot connect at the recorded address the backend calls `DescribeInstances`
for the current address, rewrites `metadata.json`, prints `instance moved to
<dns>; retrying`, and tries once more. A command the instance ran and rejected
is not retried; a genuinely unreachable instance costs two attempts, not a
loop.

## GetState

`GetState(name)` reads `metadata.json` and asks AWS for the instance state,
mapped to the status layer's vocabulary:

| AWS state | Reported |
|---|---|
| `running` | `running` |
| `stopped`, `stopping` | `stopped` |
| `pending` | `pending` |
| `shutting-down`, `terminated` | `none` |
| anything else | `unknown` |

No `metadata.json` is reported as `none`. An AWS lookup that fails (for
example, missing credentials) is reported as `unknown`, so `status` degrades
rather than fails.

## CopyCredentials

`CopyCredentials(name, credentials)` copies the host's Claude credentials to
the instance over SSH. It overwrites the instance's copy only when the host's
is fresher, so a session running on the instance that refreshed its own
credentials keeps them.

## Wipe

`isolarium ec2 wipe` (`internal/ec2/wipe.go`) is not part of the `Backend`
interface. It destroys the VPC, subnet, security group, and key pair that
every EC2 environment shares, and removes the host's SSH material. It refuses
while any `instance-<name>.tf` exists, listing the `destroy` commands to run
first. The state bucket is retained and its name is reported.
