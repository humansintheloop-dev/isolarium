# Container Configuration via pid.yaml — Specification

## Purpose and Background

Isolarium provides isolated execution environments (VM, container, nono) for autonomous coding agents. Currently, the embedded Dockerfile (`internal/docker/Dockerfile`) provides a fixed set of tools (Node.js, Java 17, Gradle, gh, Claude Code, uv). Projects that need additional tools — or project-specific setup like MCP server configuration — have no declarative way to customize the environment.

The e2e test scripts for gradlew and pytest (`test-scripts/test-end-to-end-with-gradlew.sh`, `test-scripts/test-end-to-end-with-pytest.sh`) are hardcoded to nono, while the claude test script (`test-scripts/test-end-to-end-with-claude.sh`) already supports `nono|container|vm|all`. Extending the gradlew/pytest scripts to work across isolation types requires a mechanism to initialize containers and VMs with project-specific tooling.

This feature introduces `pid.yaml` — a project-level configuration file that declaratively specifies environment setup scripts — along with an `--env` flag for passing environment variables and `ISOLARIUM_NAME`/`ISOLARIUM_TYPE` env var support.

## Target Users and Personas

**Project author** — A developer who maintains a repository that uses isolarium for isolated execution. They write `pid.yaml` to declare what tools and configuration their project needs in each isolation type.

**CI/CD operator** — Runs isolarium commands in automation, passing secrets via `--env` flags and `.env.local` files.

## Problem Statement and Goals

**Problem:** There is no way to customize the container or VM environment per-project. Projects needing tools beyond the base image (e.g., Go, golangci-lint, codescene CLI) cannot run their full workflow in containers or VMs.

**Goals:**
1. Projects can declaratively specify environment initialization via `pid.yaml`.
2. The `isolarium create` command processes `pid.yaml` as part of environment creation.
3. Environment variables can be passed to any isolarium subcommand via `--env`.
4. Host scripts can call `isolarium` subcommands without repeating `--name`/`--type` flags.
5. The gradlew and pytest e2e test scripts support multiple isolation types.

## In-Scope

- `pid.yaml` parsing and processing during `isolarium create`
- `isolation_scripts`: baked into Dockerfile as `RUN` layers (containers), executed via limactl shell (VMs)
- `host_scripts`: executed on the host after environment creation
- `--env VAR` and `--env VAR=VALUE` persistent flag on root command
- `ISOLARIUM_NAME` / `ISOLARIUM_TYPE` env var defaults for `--name`/`--type` flags
- Container and VM support (both in scope)
- Extending gradlew/pytest e2e test scripts and Go test files to support isolation type parameter
- Self-test: running isolarium's own pre-commit hooks in a container and VM

## Out-of-Scope

- Custom Dockerfiles (projects cannot replace the embedded base Dockerfile)
- Nono backend support in `pid.yaml` (nono has host access, no initialization needed)
- Re-running pid.yaml scripts after create (no `isolarium setup` or similar)
- Caching or deduplication of isolation_scripts across creates

## Functional Requirements

### FR-1: pid.yaml Schema and Location

The file `pid.yaml` is located in the project repo root (the directory passed as `--work-directory` or the current working directory). It is optional. When absent, or when it contains no entry for the current `--type`, `isolarium create` behaves exactly as today.

Schema:

```yaml
isolarium:
  container:
    isolation_scripts:
      - path: scripts/container/install-go.sh
      - path: scripts/container/install-linters.sh
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

Script entries are objects with:
- `path` (required) — Path to the script, relative to the project repo root (where `pid.yaml` lives).
- `env` (optional) — List of environment variable names the script needs. Values are read from the process environment (which includes variables loaded from `.env.local` by isolarium's `--env-file` mechanism). If a declared env var is not set, create fails with an error listing the missing variable.

### FR-2: Container Create Sequence

When `isolarium create --type container` is invoked and `pid.yaml` exists with a `container` section:

1. **Read `pid.yaml`** from the work directory.
2. **Generate Dockerfile** — Start with the embedded base Dockerfile content. For each script in `isolation_scripts`, append:
   ```dockerfile
   COPY <script-filename> /tmp/<script-filename>
   RUN chmod +x /tmp/<script-filename> && /tmp/<script-filename>
   ```
   These are appended after the `WORKDIR /home/isolarium/repo` line and before the `CMD ["sleep", "infinity"]` line.
3. **Prepare build context** — Write the generated Dockerfile to the temp build context directory. Copy each referenced isolation_script file from the project repo into the same temp directory.
4. **Build image** — Run `docker build` with `--build-arg` for each env var declared across all `isolation_scripts` entries. Values are read from the process environment. The Dockerfile scripts can reference these via `ARG` declarations.
5. **Start container** — Run `docker run` as today.
6. **Write metadata** — As today.
7. **Run host_scripts** — Execute each script in order. Set `ISOLARIUM_NAME` and `ISOLARIUM_TYPE` env vars. Each host_script's declared `env` vars are also set in the script's environment (values read from the process environment).

If any isolation_script fails (non-zero exit from `docker build`), create fails. The partially built image is left for debugging.

If any host_script fails (non-zero exit), create fails. The container is left running for debugging.

### FR-3: VM Create Sequence

When `isolarium create --type vm` is invoked and `pid.yaml` exists with a `vm` section:

1. **Create/start VM** — As today (`createAndSetupVM`).
2. **Read `pid.yaml`** from the work directory.
3. **Run isolation_scripts** — Execute each script via `limactl shell` (or the existing lima exec mechanism). Each script's declared `env` vars are passed as environment variables (values read from the process environment).
4. **Run host_scripts** — Same as container: set `ISOLARIUM_NAME`/`ISOLARIUM_TYPE`, set each script's declared `env` vars, execute in order.

If any script fails, create fails but the VM is left running for debugging.

### FR-4: --env Persistent Flag

Add a repeatable `--env` persistent flag on the root command for ad-hoc runtime env var passing:

```
isolarium --env CS_ACCESS_TOKEN run --type container -- claude
isolarium --env CS_ACCESS_TOKEN=myvalue shell --type vm
```

Two forms:
- `--env VAR` — Read the value of `VAR` from the current process environment (`os.Getenv("VAR")`).
- `--env VAR=VALUE` — Use the literal value.

Primary use case is `run` / `shell`: each `--env` var is added to the `envVars` map passed to `Backend.Exec`/`ExecInteractive`/`OpenShell`. For containers, these become `-e` flags on `docker exec`. For VMs, they become environment variables in `limactl shell`.

For `create`, env vars needed by scripts are declared in `pid.yaml` (see FR-1) and read from the process environment automatically. The `--env` flag is not the primary mechanism for create-time env vars.

The `--env` flag is parsed in `PersistentPreRunE` (or as a persistent flag variable) and stored as a `map[string]string` accessible to all subcommands.

### FR-5: ISOLARIUM_NAME / ISOLARIUM_TYPE Environment Variable Defaults

When resolving the `--name` flag default: if the `--name` flag is not explicitly set and `ISOLARIUM_NAME` is set in the environment, use `ISOLARIUM_NAME`.

When resolving the `--type` flag default: if the `--type` flag is not explicitly set and `ISOLARIUM_TYPE` is set in the environment, use `ISOLARIUM_TYPE`.

This is implemented in the flag default resolution, before `PersistentPreRunE`. The precedence order is:
1. Explicit `--name`/`--type` flag on the command line
2. `ISOLARIUM_NAME`/`ISOLARIUM_TYPE` environment variables
3. Existing defaults (`lima.GetVMName()` for name, `"vm"` for type)

### FR-6: Extend Gradlew E2E Test Script

Modify `test-scripts/test-end-to-end-with-gradlew.sh` to accept isolation type arguments (`nono|container|vm|all`), following the same pattern as `test-scripts/test-end-to-end-with-claude.sh`.

Add new Go e2e test functions:
- `TestGradlewBuildInContainer_EndToEnd` — Runs `isolarium --type container run -- ./gradlew clean build` in the `testdata/spring-boot-app` directory.
- `TestGradlewBuildInVM_EndToEnd` — Same for VM.

The spring-boot-app needs Java 17 and Gradle, which are already in the base container image. Its `pid.yaml` can be empty or omitted.

### FR-7: Extend Pytest E2E Test Script

Modify `test-scripts/test-end-to-end-with-pytest.sh` to accept isolation type arguments, same pattern as the claude test script.

Add new Go e2e test functions:
- `TestPytestInContainer_EndToEnd` and `TestGreeterCliInContainer_EndToEnd`
- `TestPytestInVM_EndToEnd` and `TestGreeterCliInVM_EndToEnd`

The python-cli-app needs uv, which is already in the base container image.

### FR-8: Pre-commit Self-Test

Add e2e tests that validate the full pid.yaml machinery by running isolarium's own pre-commit hooks in both a container and a VM:

**Container variant:**

1. Ensure `CS_ACCESS_TOKEN` and `CS_ACE_ACCESS_TOKEN` are in the environment (e.g., via `.env.local`).
2. `isolarium --type container create` from the isolarium repo root.
3. The isolarium repo's `pid.yaml` defines container `isolation_scripts` that install: `go`, `golangci-lint`, `pre-commit`, and the codescene CLI (`run-codescene.sh`). The codescene script declares `env: [CS_ACCESS_TOKEN, CS_ACE_ACCESS_TOKEN]`.
4. Make a harmless file change (e.g., add a comment to a `.go` file).
5. Run `isolarium --type container --env CS_ACCESS_TOKEN --env CS_ACE_ACCESS_TOKEN run -- pre-commit run --all-files`.
6. Verify pre-commit passes (exit code 0).

**VM variant:**

1. Ensure tokens are in the environment.
2. `isolarium --type vm create` from the isolarium repo root.
3. The isolarium repo's `pid.yaml` defines vm `isolation_scripts` that install the same tools (using VM-appropriate methods). The codescene script declares its env vars.
4. Make a harmless file change.
5. Run `isolarium --type vm --env CS_ACCESS_TOKEN --env CS_ACE_ACCESS_TOKEN run -- pre-commit run --all-files`.
6. Verify pre-commit passes (exit code 0).

Both tests require `CS_ACCESS_TOKEN` and `CS_ACE_ACCESS_TOKEN` secrets to be set in the process environment or `.env.local`.

## Security Requirements

All operations are local CLI commands run by the project author or CI/CD operator. There are no multi-user authorization concerns.

- **pid.yaml scripts execute with the same privileges as the isolarium process.** Host_scripts run as the current user on the host. Isolation_scripts run inside the container (as the `isolarium` user with `sudo` access) or VM.
- **Environment variable values** (whether from pid.yaml `env` declarations or `--env` flags) do not persist anywhere — they are passed as `--build-arg` or `-e` flags to docker commands, or as environment variables to shell processes.
- **Script paths in pid.yaml are relative to the project root** and must not escape the project directory (no `../` traversal above the project root). Validate that resolved script paths are within the project root.
- **No secrets in Dockerfile layers.** Environment variables passed as `--build-arg` are visible in `docker history`. The spec acknowledges this trade-off: build-time secrets that must be hidden should use Docker BuildKit secrets or host_scripts instead. For the current use cases (tool installation), build-args are acceptable.

## Non-Functional Requirements

- **Backward compatibility:** All changes are additive. Existing `isolarium create` behavior is unchanged when `pid.yaml` is absent.
- **Docker layer caching:** Isolation_scripts appended as `RUN` layers benefit from Docker's build cache. Unchanged scripts don't re-execute on subsequent builds.
- **Error visibility:** Script output (stdout/stderr) is displayed to the user during create, so failures are diagnosable.
- **Fail-fast with debuggability:** Script failures halt the create sequence immediately but leave the environment intact for inspection.

## Success Metrics

1. The gradlew and pytest e2e test scripts accept isolation type parameters and pass for `container` and `vm`.
2. The pre-commit self-test passes in both a container and a VM with codescene tokens provided.
3. Existing e2e tests (nono, container, VM for claude) continue to pass unchanged.

## Epics and User Stories

### Epic 1: pid.yaml Configuration

**US-1.1:** As a project author, I can create a `pid.yaml` in my repo root that specifies `isolation_scripts` for the `container` type, so that `isolarium create --type container` installs my project's required tools into the Docker image.

**US-1.2:** As a project author, I can specify `host_scripts` in `pid.yaml` that run on the host after environment creation, so that I can configure secrets and MCP servers that require host access.

**US-1.3:** As a project author, I can specify different scripts for `container` and `vm` types in `pid.yaml`, so that each environment gets the appropriate installation commands.

**US-1.4:** As a project author, when `pid.yaml` is absent or has no entry for my `--type`, `isolarium create` works exactly as before, so there is no breaking change.

### Epic 2: Environment Variable Passing

**US-2.1:** As a project author, I can declare `env` vars per-script in `pid.yaml`, so that `isolarium create` automatically reads required secrets from the process environment without needing `--env` flags on the command line.

**US-2.2:** As a CI/CD operator, I can pass `--env VAR` to `isolarium run`/`shell` so that secrets are available at runtime inside the container or VM.

**US-2.3:** As a CI/CD operator, I can pass `--env VAR=VALUE` with an explicit value to `run`/`shell`, so that I can override or provide values not in my environment.

### Epic 3: ISOLARIUM_NAME / ISOLARIUM_TYPE Env Vars

**US-3.1:** As a host_script author, I can call `isolarium shell -- <command>` without specifying `--name` and `--type`, because the environment variables `ISOLARIUM_NAME` and `ISOLARIUM_TYPE` are set by isolarium before my script runs.

### Epic 4: E2E Test Expansion

**US-4.1:** As a developer, I can run `test-end-to-end-with-gradlew.sh container` to execute the gradlew build e2e test in a container.

**US-4.2:** As a developer, I can run `test-end-to-end-with-pytest.sh container` to execute the pytest e2e tests in a container.

**US-4.3:** As a developer, I can run a pre-commit self-test that creates a container for the isolarium repo, makes a harmless change, and runs all pre-commit hooks including codescene.

**US-4.4:** As a developer, I can run a pre-commit self-test that creates a VM for the isolarium repo, makes a harmless change, and runs all pre-commit hooks including codescene.

## Scenarios

### Scenario 1: Gradlew Build in Container (Minimal pid.yaml)

1. Developer runs `test-end-to-end-with-gradlew.sh container`.
2. Script builds the isolarium binary and runs `TestGradlewBuildInContainer_EndToEnd`.
3. Test invokes `isolarium --type container create` from `testdata/spring-boot-app/`.
4. No `pid.yaml` (or empty one) — base image has Java 17 and Gradle.
5. Test invokes `isolarium --type container run -- ./gradlew clean build`.
6. Output contains "BUILD SUCCESSFUL".

### Scenario 2: Pre-commit in Container (Full pid.yaml with Secrets)

This is the primary end-to-end scenario that exercises all pid.yaml features.

1. The isolarium repo root contains `pid.yaml`:
   ```yaml
   isolarium:
     container:
       isolation_scripts:
         - path: scripts/container/install-go.sh
         - path: scripts/container/install-linters.sh
         - path: scripts/container/install-pre-commit.sh
         - path: scripts/container/install-codescene.sh
           env:
             - CS_ACCESS_TOKEN
             - CS_ACE_ACCESS_TOKEN
   ```
2. Developer ensures `CS_ACCESS_TOKEN` and `CS_ACE_ACCESS_TOKEN` are in the environment (e.g., via `.env.local`).
3. Developer runs `isolarium --type container create`.
4. Isolarium reads `pid.yaml`, collects declared env vars, validates they are set, generates Dockerfile appending `RUN` layers for each script, copies scripts into build context.
5. `docker build` runs with `--build-arg CS_ACCESS_TOKEN=<value> --build-arg CS_ACE_ACCESS_TOKEN=<value>`.
6. Container starts.
7. Developer runs `isolarium --type container --env CS_ACCESS_TOKEN --env CS_ACE_ACCESS_TOKEN run -- pre-commit run --all-files`.
8. Pre-commit runs all hooks (shellcheck, gitleaks, codescene, go vet, golangci-lint, precommit-check).
9. All hooks pass (exit code 0).

### Scenario 3: Pytest in Container

1. Developer runs `test-end-to-end-with-pytest.sh container`.
2. Test invokes `isolarium --type container create` from `testdata/python-cli-app/`.
3. No pid.yaml needed — base image has uv.
4. Test invokes `isolarium --type container run -- uv run pytest -v`.
5. Output contains "2 passed".

### Scenario 4: Host Script with Secrets

1. A project's `pid.yaml` includes a `host_scripts` entry:
   ```yaml
   isolarium:
     container:
       host_scripts:
         - path: scripts/setup-mcp.sh
           env:
             - CS_ACCESS_TOKEN
             - CS_ACE_ACCESS_TOKEN
   ```
2. Developer ensures tokens are in the environment (e.g., via `.env.local`).
3. Developer runs `isolarium --type container create`.
4. Container is built and started (no isolation_scripts in this example).
5. Isolarium sets `ISOLARIUM_NAME=isolarium-container`, `ISOLARIUM_TYPE=container`, `CS_ACCESS_TOKEN=<value>`, and `CS_ACE_ACCESS_TOKEN=<value>`, then executes `scripts/setup-mcp.sh`.
6. The script runs `isolarium shell -- claude mcp add codescene --env CS_ACCESS_TOKEN=$CS_ACCESS_TOKEN -- cs-mcp`.
7. Because `ISOLARIUM_NAME` and `ISOLARIUM_TYPE` are set, the `isolarium shell` call targets the correct container without explicit flags.

### Scenario 5: Pre-commit in VM (Full pid.yaml with Secrets)

1. The isolarium repo root contains `pid.yaml` with a `vm` section:
   ```yaml
   isolarium:
     vm:
       isolation_scripts:
         - path: scripts/vm/install-go.sh
         - path: scripts/vm/install-linters.sh
         - path: scripts/vm/install-pre-commit.sh
         - path: scripts/vm/install-codescene.sh
           env:
             - CS_ACCESS_TOKEN
             - CS_ACE_ACCESS_TOKEN
   ```
2. Developer ensures tokens are in the environment (e.g., via `.env.local`).
3. Developer runs `isolarium --type vm create`.
4. VM is created and started as today.
5. Isolarium reads `pid.yaml` and executes each `isolation_scripts` entry via limactl shell, passing declared env vars.
6. Developer runs `isolarium --type vm --env CS_ACCESS_TOKEN --env CS_ACE_ACCESS_TOKEN run -- pre-commit run --all-files`.
7. Pre-commit runs all hooks.
8. All hooks pass (exit code 0).

## Change History

### 2026-03-10: Add VM pre-commit scenario

Added VM variant of the pre-commit self-test (FR-8, US-4.4, Scenario 5). The pre-commit self-test must validate pid.yaml machinery for both container and VM isolation types, not just containers.

### 2026-03-10: Declare env vars in pid.yaml instead of --env on create

Moved create-time env var declarations from `--env` command-line flags into pid.yaml per-script `env` lists. This eliminates the tedious `isolarium --type vm --env CS_ACCESS_TOKEN --env CS_ACE_ACCESS_TOKEN create` pattern. Now `isolarium --type vm create` is sufficient — env vars are read from the process environment automatically based on pid.yaml declarations. The `--env` flag remains for ad-hoc use on `run`/`shell`.
