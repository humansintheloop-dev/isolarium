# Secure coding agent runner on macOS - Option 5

## Overview
Option 5 combines a hardened Linux VM with a **repo-scoped machine identity**.
It is the most defensible way to run autonomous coding agents on macOS.

The core idea is:
- The agent runs inside a disposable Linux VM
- The repository is cloned inside the VM
- Credentials are short-lived and limited to a single repository
- GitHub enforces the access boundary, not local configuration

## Architecture summary
- Host OS: macOS (Apple Silicon)
- Virtualization: Linux VM (Lima)
- Container runtime: Docker inside the VM
- Repository access: GitHub App installation token
- Credential lifetime: Short-lived (minutes to hours)
- Reset model: Delete and recreate the VM

## Threat model assumptions
This option assumes:
- The agent may behave incorrectly or unexpectedly
- The agent may execute untrusted dependencies
- Docker access implies full control of the VM
- VM compromise is acceptable
- Host compromise is not acceptable

## VM isolation model
### VM boundary
- A real Linux kernel provides isolation from macOS
- No host filesystem mounts are required
- No host Docker socket is exposed

### Disposability
- The VM is treated as disposable
- Recovery from compromise is done by deleting the VM
- No long-lived state is trusted

## Repository handling
### Clone inside the VM
- The repository is cloned directly inside the VM filesystem
- No source code is mounted from the host
- This eliminates accidental access to host files

### No ambient Git credentials
- No personal Git credentials are present
- No global git configuration is shared from the host

## GitHub App identity
### What the agent is
- The agent authenticates as a GitHub App
- The App is installed on exactly one repository
- The App is not a GitHub user

### Permissions
Typical minimal permissions:
- Contents: Read / Write
- Pull requests: Read / Write (only if the agent opens PRs)

All other permissions are denied.

### Token properties
- Tokens are minted at runtime
- Tokens are short-lived
- Tokens are scoped to the installed repository only

If a token leaks:
- It expires automatically
- It cannot access any other repository

## Credential provisioning
### Runtime injection
- Tokens are injected at runtime only
- Tokens are passed via environment variables or stdin
- Tokens are not baked into the VM image
- Tokens are not committed to disk when possible

### Separation of identities
- The GitHub App represents the agent
- The human developer identity is never exposed to the VM
- Audit logs clearly distinguish agent actions from human actions

## Docker usage
- Docker runs inside the VM
- Testcontainers and Docker Compose operate normally
- Docker control is scoped to the VM only

The agent is assumed to fully control Docker inside the VM.

## Audit and compliance properties
- All repository access is attributable to the GitHub App
- All actions are logged by GitHub
- Permissions are explicit and reviewable
- Blast radius is limited to a single repository

## Operational workflow
### Typical run
1. Create or start the Lima VM
2. Mint a GitHub App installation token
3. Clone the repository inside the VM
4. Inject the token and model API key at runtime
5. Run the coding agent
6. Review outputs and commits
7. Delete the VM if a clean reset is required

### Failure handling
- If anything looks suspicious, delete the VM
- Recreate the VM from a known configuration
- Rotate credentials if necessary

## When to choose this option
This option is appropriate when:
- The agent is autonomous
- The repository contains valuable IP
- You want a defensible security story
- You want strict blast-radius control
- You want clean audit trails

## Summary
Option 5 provides:
- Strong isolation via a real VM
- Minimal ambient authority
- Repo-level access enforcement by GitHub
- Short-lived, auditable credentials
- A clear and explainable security boundary

It is the strongest practical option for running coding agents securely on macOS.
