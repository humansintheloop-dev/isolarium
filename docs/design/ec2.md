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
| `isolarium run --type ec2 --name <name> -- <cmd>` | `Exec` (inside the tmux session; a re-run of the same command reattaches) |
| `isolarium run --type ec2 --name <name> --create -- <cmd>` | `GetState`, then `Create` when it answers `none`, then `Exec` |
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
9. Wait for SSH, then wait for cloud-init to finish. `cloud-init status
   --wait --long` exits 0 for a clean run, 2 for a degraded one (every module
   ran, something logged a warning — which the Ubuntu AMI's IMDS probe over
   IPv6 does on every instance without IPv6) and 1 when a module failed. A
   degraded run counts as ready and its warnings are relayed on stderr; a
   failed one fails the create with the modules named.
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

`Exec(req)` runs a non-interactive command in the repository directory on the
instance, inside the instance's tmux session, and returns the command's own
exit status. The address comes from `metadata.json`, so no AWS or Terraform
call is made.

The command travels over `ssh -tt`, which forces a pseudo-terminal on the
instance even when isolarium itself has no terminal, with the host's stdin left
disconnected (`ec2.ExecInSessionCommand`). That is what lets tmux start under
i2code, and what keeps the command running if the connection drops.

- Whether a session is running is asked with `tmux has-session -t <session>
  2>/dev/null` over the plain transport; only its exit status is wanted, and
  tmux's complaint that no server is running — the ordinary first-run case —
  is dropped on the instance rather than shown to the user.
- If no session is running, `Exec` first clears the session's status file
  (`mkdir -p ~/.isolarium && rm -f ~/.isolarium/status-<session>`, over the
  capture transport) and then starts one: `tmux new-session -s <session> -- sh
  -c '<cmd>; echo $? > ~/.isolarium/status-<session>; sleep 1' \; set-option
  -t <session> @isolarium-command '<cmd>'`. The `sh -c` wrapper
  (`ec2.WrapWithExitStatus`) records the command's exit status when it ends,
  because the tmux client exits 0 whatever the command did, and then holds the
  pane open for a second: tmux tears the window down the moment its process
  exits, and the client that started the session draws the pane only a few
  milliseconds after attaching, so a command that ends inside that window —
  `echo ok` — would otherwise stream nothing. The second tmux command records
  the unwrapped arguments on the session as a tmux user option, rendered by
  `ec2.CommandRecord` (arguments quoted only where the shell would split them;
  environment excluded, since the per-run token always differs).
- If the session is already running, `Exec` reads `@isolarium-command` back
  with `tmux show-option -qv`. When it equals the arguments about to run,
  `Exec` prints `reattaching to session '<session>', which is already running:
  <cmd>` on stderr and runs `tmux attach-session -t <session>`, streaming the
  session until it ends. The status file is not cleared on this path. When it
  differs — or the session recorded nothing, because `run -i` or `shell`
  started it — `Exec` runs nothing and fails with `cannot run '<cmd>': session
  '<session>' on the instance is already running '<recorded>'; reattach to it
  with isolarium shell --type ec2, or start another session with
  --new-session`.
- Once the tmux client returns, on either path, `Exec` reads the status file
  with `cat` over the capture transport (`ec2.InstanceQuery.ReadExitStatus`)
  and returns the number it holds, leaving the file in place so a run and a
  reattached run of the same command both report the real status. A missing or
  unparsable file — the command was killed, or the instance rebooted — is an
  error naming the path, never success. So `isolarium run --type ec2 -- sh -c
  'exit 3'` exits 3, as it does for the other isolation types.
- The clear reports ssh's connect failure (exit 255) as `ErrSSHConnect`, so a
  moved instance still gets the one address refresh and retry in `onInstance`;
  the status read deliberately does not, because a retry after the client
  returned would run the command again.
- `--new-session` (`UseNewSession`) resolves a free session name first, as it
  does for `ExecInteractive`, so the new command never meets a running one.

The transports create uses internally — the SSH and cloud-init readiness
probes, `PlaceRepository`, the `pid.yaml` script runner, and the session probes
— stay on the plain transport (`ExecFunc`, `CaptureFunc`), without tmux and
without a forced pseudo-terminal.

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

## Running under i2code

i2code drives an isolated implementation run as

```
isolarium --name i2code-<idea> --type ec2 run --create -- i2code --with-sdkman implement --isolated <idea dir> ...
```

It is non-interactive by default (`--interactive` only when asked for), and
i2code starts it with `Popen(start_new_session=True)`, so isolarium has no
terminal of its own. Three consequences follow.

- `--create` launches the environment when `GetState` answers `none`
  (`createEC2IfNeeded` in `internal/cli/cmd_run.go`), paying the multi-minute
  cold start only on the first run, and then calls `Exec`.
- The command runs in tmux regardless of `--interactive`. `i2code implement`
  runs for a long time; over plain SSH it would die with the connection, and
  nothing could rejoin it. Forcing the pseudo-terminal (`-tt`) is what makes
  tmux start when there is no host terminal to inherit one from.
- A re-run of the same command reattaches. `Exec` records the command on the
  session and compares it on the next run, so i2code's retry finds the run it
  started rather than starting a second one beside it, and streams its output
  until it ends. A run with a different command is refused while that session
  is running.

To reattach by hand, run `isolarium shell --type ec2 --name i2code-<idea>`,
which joins the same tmux session (`isolarium` unless `--new-session` chose
another), or `ssh` to the instance and run `tmux attach-session -t isolarium`.
If you also run tmux locally, the prefix key is `Ctrl-b Ctrl-b`.

One caveat: SSH ingress is pinned to the host's public `/32`, resolved by
`create` and `destroy` only. A host that moves network after the instance was
created cannot connect until the ingress rule is re-applied, so a re-run from
a different address fails to connect rather than reattaching.

## Wipe

`isolarium ec2 wipe` (`internal/ec2/wipe.go`) is not part of the `Backend`
interface. It destroys the VPC, subnet, security group, and key pair that
every EC2 environment shares, and removes the host's SSH material. It refuses
while any `instance-<name>.tf` exists, listing the `destroy` commands to run
first. The state bucket is retained and its name is reported.
