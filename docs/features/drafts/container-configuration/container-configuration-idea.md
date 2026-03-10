# Container Configuration via pid.yaml

## Summary

Introduce a project-level configuration file (`pid.yaml`) that defines how isolated environments (container or VM) are initialized. This enables e2e test scripts to work across isolation types and allows projects to declaratively specify their environment setup.

## Motivation

The e2e test scripts for gradlew and pytest are hardcoded to nono, while the claude test script supports `nono|container|vm|all`. To run these tests in containers or VMs, the environments need project-specific setup (tool installation, secrets configuration). `pid.yaml` provides a declarative way to define this setup per isolation type.

## pid.yaml

Located in the project repo root. Optional - when absent, `isolarium create` behaves as today.

```yaml
isolarium:
    container:
        isolation_scripts:
            - path: scripts/container/install-go.sh
            - path: scripts/container/install-codescene.sh
              env:
                  - CS_ACCESS_TOKEN
                  - CS_ACE_ACCESS_TOKEN
        host_scripts:
            - path: scripts/setup-mcp.sh
              env:
                  - CS_ACCESS_TOKEN
    vm:
        isolation_scripts:
            - path: scripts/vm/install-go.sh
            - path: scripts/vm/install-codescene.sh
              env:
                  - CS_ACCESS_TOKEN
                  - CS_ACE_ACCESS_TOKEN
```

Script entries have `path` (required) and `env` (optional list of env var names needed by the script). Values are read from the process environment (including `.env.local`).

### isolation_scripts

- Run inside the isolated environment during `isolarium create`
- For containers: baked into the Docker image as `RUN` layers appended to the embedded base Dockerfile. Scripts are copied into the temp build context. Declared `env` vars become `--build-arg`.
- For VMs: executed via limactl shell after VM creation. Declared `env` vars passed through.

### host_scripts

- Run on the host after the environment is created
- Have access to host secrets and resources
- `ISOLARIUM_NAME` and `ISOLARIUM_TYPE` env vars are set, so scripts can call `isolarium shell ...` without repeating flags
- Each script's declared `env` vars are set from the process environment

### Create sequence

**Container:** 1) Build Docker image (base + isolation_scripts as RUN layers), 2) Start container, 3) Run host_scripts

**VM:** 1) Create/start VM, 2) Run isolation_scripts via limactl shell, 3) Run host_scripts

### Error handling

If any script fails, create fails but leaves the environment in place for debugging.

## --env flag

A persistent flag on the root command (`--env VAR` or `--env VAR=VALUE`), primarily for `run`/`shell` commands to pass ad-hoc runtime environment variables. For `create`, env vars are declared per-script in `pid.yaml` and read from the process environment automatically.

## ISOLARIUM_NAME / ISOLARIUM_TYPE env vars

New behavior: isolarium reads these env vars as defaults for `--name`/`--type` flags. This enables host_scripts (and other tooling) to call `isolarium` subcommands without repeating flags.

## Example: codescene MCP setup

```shell
brew tap codescene-oss/codescene-mcp-server https://github.com/codescene-oss/codescene-mcp-server
brew install cs-mcp

claude mcp add codescene \
    --env CS_ACCESS_TOKEN=$CS_ACCESS_TOKEN \
    --env CS_ACE_ACCESS_TOKEN=$CS_ACE_ACCESS_TOKEN \
    -- \
    cs-mcp
```

Note: for a linux container the install commands would differ (apt-get vs brew).

## Steel thread validation

1. **Isolarium self-test**: Create an isolated environment for the isolarium repo itself, make a harmless file change, and trigger pre-commit hooks. This exercises isolation_scripts (installing go, golangci-lint, codescene CLI, pre-commit) and --env (passing CS_ACCESS_TOKEN/CS_ACE_ACCESS_TOKEN).

2. **Gradlew/pytest e2e tests**: Add isolation type parameter to `test-end-to-end-with-gradlew.sh` and `test-end-to-end-with-pytest.sh` (and corresponding Go e2e tests). Spring-boot-app works with the base image as-is, so its pid.yaml would be minimal.
