

Now I have all the skill guidance loaded. Let me generate the plan based on the spec, idea, discussion, and all skill requirements.

# Container Configuration via pid.yaml — Implementation Plan

## Idea Type

**A. User-facing feature** — `pid.yaml` is a configuration mechanism that project authors use directly to define how their isolated environments are set up.

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

## Overview

This plan implements `pid.yaml` support for isolarium — a project-level configuration file that declares how isolated environments (container/VM) are initialized with project-specific tooling. The implementation proceeds through steel threads ordered by causal dependency:

1. **pid.yaml parsing + ISOLARIUM_NAME/TYPE env vars** — Core config loading and env var defaults (foundation for everything else)
2. **Container isolation_scripts** — Dockerfile generation from pid.yaml (enables container customization)
3. **--env flag for run/shell** — Ad-hoc runtime env var passing
4. **Host scripts** — Post-create host-side execution with ISOLARIUM_NAME/TYPE
5. **VM isolation_scripts** — VM-side script execution via limactl
6. **Gradlew e2e in container** — Validate base container works for existing testdata
7. **Pytest e2e in container** — Validate base container works for python testdata
8. **Pre-commit self-test in container** — Full pid.yaml validation with real isolation_scripts and secrets
9. **Pre-commit self-test in VM** — Full pid.yaml validation for VM backend

## Key Architecture Notes

- **Go project** — All production code is Go, tests use `go test`
- **Existing backends** — Container (Docker) and VM (Lima) backends already exist behind a `Backend` interface
- **Embedded Dockerfile** — `internal/docker/Dockerfile` is the base; pid.yaml isolation_scripts append `RUN` layers
- **CLI** — Cobra-based CLI with root, create, run, shell subcommands
- **Existing e2e pattern** — `test-scripts/test-end-to-end-with-claude.sh` already supports `nono|container|vm|all` — follow this pattern

---

## Steel Thread 1: pid.yaml Parsing and ISOLARIUM_NAME/TYPE Env Var Defaults

This thread establishes the foundation: parsing `pid.yaml` and supporting `ISOLARIUM_NAME`/`ISOLARIUM_TYPE` environment variable defaults for `--name`/`--type` flags. These are prerequisites for all subsequent threads.

- [x] **Task 1.1: Parse pid.yaml and return typed configuration**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/config/...`
  - Observable: A `LoadPidConfig(workDir string)` function reads `pid.yaml` from the given directory, returns a typed `PidConfig` struct with `Container` and `VM` sections each containing `IsolationScripts` and `HostScripts` (each script has `Path` and `Env` fields). Returns nil config (no error) when file is absent.
  - Evidence: Unit tests verify: (1) valid pid.yaml parses to correct struct, (2) missing file returns nil config without error, (3) missing required `path` field returns error, (4) script paths with `../` traversal above project root return error
  - Steps:
    - [x] Create `internal/config/pidconfig.go` with `PidConfig`, `IsolationTypeConfig`, `ScriptEntry` types and `LoadPidConfig()` function
    - [x] Create `internal/config/pidconfig_test.go` with tests using embedded YAML strings and `t.TempDir()`
    - [x] Validate that resolved script paths do not escape the project root (no `../` traversal above work directory)

- [x] **Task 1.2: ISOLARIUM_NAME and ISOLARIUM_TYPE env vars set flag defaults**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./cmd/...`
  - Observable: When `--name` flag is not explicitly set and `ISOLARIUM_NAME` env var is set, the name defaults to `ISOLARIUM_NAME`. Same for `--type`/`ISOLARIUM_TYPE`. Explicit flags take precedence over env vars. Env vars take precedence over existing defaults.
  - Evidence: Unit tests verify precedence: (1) explicit flag overrides env var, (2) env var overrides default, (3) absent env var falls back to existing default
  - Steps:
    - [x] Find where `--name` and `--type` flag defaults are resolved in the root command (likely `cmd/root.go`)
    - [x] Add env var lookup: if flag not explicitly set, check `os.Getenv("ISOLARIUM_NAME")` / `os.Getenv("ISOLARIUM_TYPE")`
    - [x] Create tests in `cmd/root_test.go` (or appropriate test file) that set env vars via `t.Setenv()` and verify flag resolution

- [x] **Task 1.3: pid.yaml parsing and env var defaults validated in CI**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-end-to-end.sh`
  - Observable: CI runs `go test ./...` which includes the pid.yaml parsing tests and env var default tests, and the existing test-end-to-end.sh passes
  - Evidence: CI workflow runs `go test ./...` and `./test-scripts/test-end-to-end.sh` and both pass
  - Steps:
    - [x] Verify existing `.github/workflows/ci.yml` already runs `go test ./...` (if not, add it)
    - [x] Run full test suite locally to confirm all existing and new tests pass

---

## Steel Thread 2: Container Isolation Scripts

This thread implements the core Dockerfile generation from pid.yaml `isolation_scripts` during `isolarium create --type container`.

- [x] **Task 2.1: Generate Dockerfile with appended RUN layers from isolation_scripts**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: A function takes the base Dockerfile content and a list of `ScriptEntry` objects, and returns a new Dockerfile with `COPY` + `RUN` layers appended for each script (inserted after `WORKDIR /home/isolarium/repo` and before `CMD ["sleep", "infinity"]`). Each script's `env` vars become `ARG` declarations before the `RUN`.
  - Evidence: Unit tests verify: (1) no isolation_scripts returns base Dockerfile unchanged, (2) one script appends correct COPY+RUN, (3) multiple scripts append in order, (4) scripts with env vars include ARG declarations
  - Steps:
    - [x] Create a `GenerateDockerfile(baseDockerfile string, scripts []config.ScriptEntry) string` function in `internal/docker/` (or extend existing Dockerfile generation)
    - [x] Create unit tests in `internal/docker/dockerfile_gen_test.go`

- [x] **Task 2.2: Container create copies isolation_scripts to build context and passes build-args**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: During container create, when pid.yaml has container `isolation_scripts`: (1) each referenced script file is copied from the project directory into the temp build context, (2) the generated Dockerfile is written to the build context, (3) `docker build` is invoked with `--build-arg` for each declared env var (values read from process environment), (4) missing declared env vars cause create to fail with an error listing the missing variable
  - Evidence: Unit/integration tests verify: (1) build context contains copied scripts, (2) docker build command includes correct --build-arg flags, (3) missing env var returns descriptive error
  - Steps:
    - [x] Find the existing container create flow (likely `internal/docker/backend.go` or similar)
    - [x] Add pid.yaml loading at the start of container create
    - [x] Implement build context preparation: copy isolation_scripts into temp dir
    - [x] Extend docker build invocation to include `--build-arg` flags
    - [x] Add env var validation: check all declared env vars are set, fail with clear error if not
    - [x] Create tests that mock or verify the docker build command construction

- [x] **Task 2.3: Container isolation_scripts e2e smoke test**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-container-isolation-scripts.sh`
  - Observable: A test creates a container from a test project with a pid.yaml that has one isolation_script (e.g., installs a small tool), then verifies the tool is available inside the container
  - Evidence: `./test-scripts/test-container-isolation-scripts.sh` passes — creates container, runs command to verify tool is installed, destroys container
  - Steps:
    - [x] Create a minimal test fixture (e.g., `testdata/pid-yaml-test/`) with a `pid.yaml` containing one `isolation_script` that installs a lightweight tool (e.g., `jq` or creates a marker file)
    - [x] Create the isolation script referenced by pid.yaml
    - [x] Create `test-scripts/test-container-isolation-scripts.sh` that: builds isolarium, runs `isolarium create --type container` from the test fixture dir, runs `isolarium run -- <verify-command>`, runs `isolarium destroy`
    - [x] Add the new test script to `test-scripts/test-end-to-end.sh`

---

## Steel Thread 3: --env Flag for Run/Shell

This thread adds the `--env` persistent flag on the root command for passing ad-hoc runtime environment variables to `run`/`shell` commands.

- [x] **Task 3.1: --env persistent flag parses VAR and VAR=VALUE forms**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./cmd/...`
  - Observable: The root command accepts `--env VAR` (reads value from `os.Getenv`) and `--env VAR=VALUE` (uses literal value). Parsed env vars are stored as a `map[string]string` accessible to subcommands.
  - Evidence: Unit tests verify: (1) `--env FOO` reads from os environment, (2) `--env FOO=bar` uses literal value, (3) multiple `--env` flags accumulate, (4) `--env VAR` where VAR is unset results in empty string or error (per spec behavior)
  - Steps:
    - [x] Add `--env` as a persistent `StringSlice` flag on the root command
    - [x] Parse each entry in `PersistentPreRunE`: split on first `=` to distinguish `VAR` from `VAR=VALUE`
    - [x] Store parsed map in a location accessible to subcommands (e.g., on a context struct or package-level variable)
    - [x] Create tests in `cmd/env_flag_test.go`

- [x] **Task 3.2: --env vars passed to container run/shell as -e flags**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/docker/...`
  - Observable: When `isolarium --env FOO=bar run -- env` is executed with container type, the `docker exec` command includes `-e FOO=bar`, and the command inside the container sees `FOO=bar` in its environment
  - Evidence: Unit tests verify the docker exec command construction includes `-e` flags for each --env var. Integration test (if feasible) runs `isolarium --env TEST_VAR=hello run -- printenv TEST_VAR` and verifies output contains `hello`.
  - Steps:
    - [x] Find where `docker exec` is constructed for `run`/`shell` (likely in the Docker backend's `Exec`/`ExecInteractive`/`OpenShell` methods)
    - [x] Pass the env vars map from the root command to the backend methods
    - [x] Add `-e KEY=VALUE` flags to the docker exec command for each entry
    - [x] Create unit tests verifying command construction

- [x] **Task 3.3: --env vars passed to VM run/shell**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/lima/...`
  - Observable: When `isolarium --env FOO=bar run -- env` is executed with VM type, the `limactl shell` command passes `FOO=bar` as an environment variable, and the command inside the VM sees it
  - Evidence: Unit tests verify the limactl shell command construction includes env var passing mechanism
  - Steps:
    - [x] Find where `limactl shell` is constructed for `run`/`shell` in the Lima backend
    - [x] Pass the env vars map to the backend methods
    - [x] Add env var flags to the limactl shell command
    - [x] Create unit tests verifying command construction

- [x] **Task 3.4: --env flag e2e smoke test**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-env-flag.sh`
  - Observable: A test passes `--env TEST_VAR=hello` to `isolarium run` in a container and verifies the variable is visible inside
  - Evidence: `./test-scripts/test-env-flag.sh` passes
  - Steps:
    - [x] Create `test-scripts/test-env-flag.sh` that: creates a container, runs `isolarium --env TEST_VAR=hello123 run --type container -- printenv TEST_VAR`, asserts output contains `hello123`, destroys container
    - [x] Add to `test-scripts/test-end-to-end.sh`

---

## Steel Thread 4: Host Scripts

This thread implements `host_scripts` execution after environment creation, with `ISOLARIUM_NAME` and `ISOLARIUM_TYPE` set in the script environment.

- [x] **Task 4.1: Host scripts execute after container create with ISOLARIUM_NAME/TYPE set**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/...`
  - Observable: After container start, if pid.yaml defines `host_scripts`, each script is executed on the host with `ISOLARIUM_NAME` and `ISOLARIUM_TYPE` env vars set, plus each script's declared `env` vars read from the process environment. Scripts execute in order. If any script fails, create fails but the container is left running.
  - Evidence: Unit tests verify: (1) host scripts are invoked with correct env vars, (2) scripts execute in declared order, (3) script failure causes create to return error, (4) declared env vars missing from process env cause create to fail before running scripts
  - Steps:
    - [x] Create a `RunHostScripts(scripts []config.ScriptEntry, workDir, name, isolationType string) error` function
    - [x] Set `ISOLARIUM_NAME`, `ISOLARIUM_TYPE`, and per-script `env` vars in each script's `exec.Cmd.Env`
    - [x] Integrate into the container create flow after container start
    - [x] Create unit tests that use real temp scripts (small shell scripts that write marker files)

- [x] **Task 4.2: Host scripts execute after VM create**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/...`
  - Observable: After VM creation and isolation_scripts (if any), host_scripts execute on the host with the same env var behavior as container host_scripts
  - Evidence: Unit tests verify host scripts are invoked during VM create with correct env vars
  - Steps:
    - [x] Integrate `RunHostScripts` into the VM create flow
    - [x] Create tests verifying VM create calls host scripts with correct parameters

- [x] **Task 4.3: Host scripts e2e smoke test**
  - TaskType: INFRA
  - Entrypoint: `./test-scripts/test-host-scripts.sh`
  - Observable: A test creates a container from a test project with pid.yaml that has a `host_script` which creates a marker file. After create, the marker file exists on the host.
  - Evidence: `./test-scripts/test-host-scripts.sh` passes
  - Steps:
    - [x] Create a test fixture with pid.yaml defining a host_script that writes a marker file (e.g., `touch /tmp/isolarium-host-script-test-marker`)
    - [x] Create the host script
    - [x] Create `test-scripts/test-host-scripts.sh` that: runs create, checks marker file exists, cleans up
    - [x] Add to `test-scripts/test-end-to-end.sh`

---

## Steel Thread 5: VM Isolation Scripts

This thread implements `isolation_scripts` for VMs — executing scripts inside the VM via limactl shell during `isolarium create --type vm`.

- [x] **Task 5.1: VM isolation_scripts execute via limactl shell during create**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/lima/...`
  - Observable: During `isolarium create --type vm`, when pid.yaml has vm `isolation_scripts`: each script is executed inside the VM via `limactl shell` (or existing lima exec mechanism). Each script's declared `env` vars are passed as environment variables. Missing declared env vars cause create to fail. Script failure causes create to fail but VM is left running.
  - Evidence: Unit tests verify: (1) limactl shell commands are constructed correctly for each script, (2) env vars are passed through, (3) missing env vars cause error, (4) script failure propagates as create error
  - Steps:
    - [x] Find the existing VM create flow (likely `internal/lima/backend.go`)
    - [x] After VM creation/start, load pid.yaml and iterate vm `isolation_scripts`
    - [x] For each script, execute via limactl shell with env vars
    - [x] Validate declared env vars are present before execution
    - [x] Create unit tests mocking the limactl shell execution

---

## Steel Thread 6: Gradlew E2E in Container

This thread extends the gradlew e2e test to support container isolation type, validating that the base container image works for Java/Gradle projects without pid.yaml customization.

- [x] **Task 6.1: Gradlew e2e test script accepts isolation type parameter**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-end-to-end-with-gradlew.sh container`
  - Observable: `test-end-to-end-with-gradlew.sh` accepts `nono|container|vm|all` arguments (following `test-end-to-end-with-claude.sh` pattern). When called with `container`, it runs the gradlew build e2e test in a container.
  - Evidence: `./test-scripts/test-end-to-end-with-gradlew.sh container` passes — creates container from `testdata/spring-boot-app/`, runs `./gradlew clean build`, output contains "BUILD SUCCESSFUL"
  - Steps:
    - [x] Read `test-scripts/test-end-to-end-with-claude.sh` to understand the isolation type argument pattern
    - [x] Modify `test-scripts/test-end-to-end-with-gradlew.sh` to accept isolation type args
    - [x] Add Go e2e test function `TestGradlewBuildInContainer_EndToEnd` in the appropriate test file
    - [x] Ensure the test creates a container, runs gradlew build, and asserts BUILD SUCCESSFUL

- [x] **Task 6.2: Gradlew e2e test in VM with no-tests-to-run safeguard**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-end-to-end-with-gradlew.sh vm`
  - Observable: `test-end-to-end-with-gradlew.sh` detects `go test`'s "no tests to run" warning and exits non-zero instead of silently passing. The gradlew VM Go test exists and passes under this hardened script.
  - Evidence: `./test-scripts/test-end-to-end-with-gradlew.sh vm` passes — creates VM from `testdata/spring-boot-app/`, runs `./gradlew clean build`, output contains "BUILD SUCCESSFUL"
  - Steps:
    - [x] Update `run_test()` in `test-end-to-end-with-gradlew.sh` to capture `go test` output and fail if it contains "no tests to run"
    - [x] Create `cmd/isolarium/e2e_gradlew_vm_test.go` with `TestGradlewBuildInVM_EndToEnd` following the pattern from `e2e_gradlew_container_test.go` but using VM isolation type
    - [x] Ensure the test creates a VM, runs gradlew build, and asserts BUILD SUCCESSFUL

---

## Steel Thread 7: Pytest E2E in Container

This thread extends the pytest e2e test to support container isolation type.

- [x] **Task 7.1: Pytest e2e test script accepts isolation type parameter**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-end-to-end-with-pytest.sh container`
  - Observable: `test-end-to-end-with-pytest.sh` accepts `nono|container|vm|all` arguments. When called with `container`, it runs pytest e2e tests in a container.
  - Evidence: `./test-scripts/test-end-to-end-with-pytest.sh container` passes — creates container from `testdata/python-cli-app/`, runs `uv run pytest -v`, output contains "2 passed"
  - Steps:
    - [x] Modify `test-scripts/test-end-to-end-with-pytest.sh` to accept isolation type args (same pattern as claude script)
    - [x] Add Go e2e test functions `TestPytestInContainer_EndToEnd` and `TestGreeterCliInContainer_EndToEnd`
    - [x] Ensure the tests create a container, run pytest/greeter CLI, and assert expected output

- [x] **Task 7.2: Pytest e2e test in VM with no-tests-to-run safeguard**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-end-to-end-with-pytest.sh vm`
  - Observable: `test-end-to-end-with-pytest.sh` detects `go test`'s "no tests to run" warning and exits non-zero instead of silently passing. The pytest VM Go tests exist and pass under this hardened script.
  - Evidence: `./test-scripts/test-end-to-end-with-pytest.sh vm` passes — creates VM from `testdata/python-cli-app/`, runs `uv run pytest -v` (output contains "2 passed") and `uv run greeter` (output contains greeting)
  - Steps:
    - [x] Update `run_test()` in `test-end-to-end-with-pytest.sh` to capture `go test` output and fail if it contains "no tests to run"
    - [x] Create `cmd/isolarium/e2e_pytest_vm_test.go` with `TestPytestInVM_EndToEnd` and `TestGreeterCliInVM_EndToEnd` following the pattern from `e2e_pytest_container_test.go` but using VM isolation type
    - [x] Ensure the tests create a VM, run pytest/greeter CLI, and assert expected output

---

## Steel Thread 8: Pre-commit Self-Test in Container

This is the primary validation thread that exercises the full pid.yaml machinery — isolation_scripts installing real tools (Go, linters, pre-commit, codescene CLI) and --env passing secrets at runtime.

- [x] **Task 8.1: Create isolarium repo's pid.yaml with container isolation_scripts**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/config/...`
  - Observable: The isolarium repo root contains a `pid.yaml` with container `isolation_scripts` entries for: `scripts/container/install-go.sh`, `scripts/container/install-linters.sh`, `scripts/container/install-pre-commit.sh`, `scripts/container/install-codescene.sh` (with `env: [CS_ACCESS_TOKEN, CS_ACE_ACCESS_TOKEN]`)
  - Evidence: Existing pid.yaml parsing tests pass with this file as input (add a test that loads the actual repo's pid.yaml)
  - Steps:
    - [x] Create `pid.yaml` in project root with the container section per spec
    - [x] Create `scripts/container/install-go.sh` — installs Go in the container
    - [x] Create `scripts/container/install-linters.sh` — installs golangci-lint, shellcheck
    - [x] Create `scripts/container/install-pre-commit.sh` — installs pre-commit
    - [x] Create `scripts/container/install-codescene.sh` — installs codescene CLI (uses `ARG CS_ACCESS_TOKEN` / `ARG CS_ACE_ACCESS_TOKEN` for build-time access if needed)
    - [x] Add a test that verifies the repo's own `pid.yaml` parses correctly

- [x] **Task 8.2: Pre-commit self-test runs all hooks in container**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-precommit-in-container.sh`
  - Observable: Creates a container for the isolarium repo using pid.yaml, makes a harmless file change, runs `pre-commit run --all-files` with codescene tokens passed via `--env`, all hooks pass (exit 0)
  - Evidence: `./test-scripts/test-precommit-in-container.sh` passes. Requires `CS_ACCESS_TOKEN` and `CS_ACE_ACCESS_TOKEN` in the environment.
  - Steps:
    - [x] Create `test-scripts/test-precommit-in-container.sh` that: (1) checks CS_ACCESS_TOKEN and CS_ACE_ACCESS_TOKEN are set, (2) runs `isolarium create --type container`, (3) makes a harmless change (e.g., add comment to a .go file), (4) runs `isolarium --env CS_ACCESS_TOKEN --env CS_ACE_ACCESS_TOKEN run --type container -- pre-commit run --all-files`, (5) asserts exit code 0, (6) cleans up
    - [x] Add to `test-scripts/test-end-to-end.sh` (conditionally, only when secrets are available)

---

## Steel Thread 9: Pre-commit Self-Test in VM

This thread validates the full pid.yaml machinery for the VM backend.

- [x] **Task 9.1: Create isolarium repo's pid.yaml VM section with isolation_scripts**
  - TaskType: INFRA
  - Entrypoint: `go test ./internal/config/...`
  - Observable: The isolarium repo's `pid.yaml` includes a `vm` section with `isolation_scripts` entries for: `scripts/vm/install-go.sh`, `scripts/vm/install-linters.sh`, `scripts/vm/install-pre-commit.sh`, `scripts/vm/install-codescene.sh` (with `env: [CS_ACCESS_TOKEN, CS_ACE_ACCESS_TOKEN]`)
  - Evidence: Existing pid.yaml parsing tests pass with the updated file
  - Steps:
    - [x] Add `vm` section to `pid.yaml` with VM-appropriate scripts
    - [x] Create `scripts/vm/install-go.sh` — installs Go in the VM (may use brew or direct download)
    - [x] Create `scripts/vm/install-linters.sh` — installs golangci-lint, shellcheck
    - [x] Create `scripts/vm/install-pre-commit.sh` — installs pre-commit
    - [x] Create `scripts/vm/install-codescene.sh` — installs codescene CLI with env var access
    - [x] Verify pid.yaml parsing tests still pass

- [x] **Task 9.2: Pre-commit self-test runs all hooks in VM**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-precommit-in-vm.sh`
  - Observable: Creates a VM for the isolarium repo using pid.yaml, makes a harmless file change, runs `pre-commit run --all-files` with codescene tokens passed via `--env`, all hooks pass (exit 0)
  - Evidence: `./test-scripts/test-precommit-in-vm.sh` passes. Requires `CS_ACCESS_TOKEN` and `CS_ACE_ACCESS_TOKEN` in the environment.
  - Steps:
    - [x] Create `test-scripts/test-precommit-in-vm.sh` following the same pattern as the container variant but using `--type vm`
    - [x] Add to `test-scripts/test-end-to-end.sh` (conditionally, only when secrets are available)

---

## Steel Thread 10: Remove Conditional Skips and Run Pre-commit Tests

The pre-commit tests in `test-end-to-end.sh` are conditionally skipped when `CS_ACCESS_TOKEN` and `CS_ACE_ACCESS_TOKEN` are not set. This means the primary validation of the pid.yaml machinery (Threads 8 and 9) silently never runs. This thread removes the conditional skips so the tests fail when secrets are missing, sets up `.env.local` in the worktree, and verifies the tests actually pass.

- [x] **Task 10.1: Pre-commit in container test passes with secrets**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-precommit-in-container.sh`
  - Observable: The conditional skip around `test-precommit-in-container.sh` in `test-end-to-end.sh` is removed so the test runs unconditionally. With `.env.local` providing `CS_ACCESS_TOKEN` and `CS_ACE_ACCESS_TOKEN`, the test creates a container, runs all pre-commit hooks including CodeScene, and passes.
  - Evidence: `./test-scripts/test-precommit-in-container.sh` exits 0. `test-end-to-end.sh` calls it unconditionally (no `SKIP` guard).
  - Steps:
    - [x] Remove the `if/else` conditional around `test-precommit-in-container.sh` in `test-end-to-end.sh`, call it unconditionally
    - [x] Ensure `.env.local` exists with `CS_ACCESS_TOKEN` and `CS_ACE_ACCESS_TOKEN`
    - [x] Run `./test-scripts/test-precommit-in-container.sh` and verify it passes
    - [x] Run `./test-scripts/test-end-to-end.sh` and verify it passes

- [x] **Task 10.2: Pre-commit in VM test passes with secrets**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-precommit-in-vm.sh`
  - Observable: The conditional skip around `test-precommit-in-vm.sh` in `test-end-to-end.sh` is removed so the test runs unconditionally. With `.env.local` providing `CS_ACCESS_TOKEN` and `CS_ACE_ACCESS_TOKEN`, the test creates a VM, runs all pre-commit hooks including CodeScene, and passes.
  - Evidence: `./test-scripts/test-precommit-in-vm.sh` exits 0. `test-end-to-end.sh` calls it unconditionally (no `SKIP` guard).
  - Steps:
    - [x] Remove the `if/else` conditional around `test-precommit-in-vm.sh` in `test-end-to-end.sh`, call it unconditionally
    - [x] Run `./test-scripts/test-precommit-in-vm.sh` and verify it passes
    - [x] Run `./test-scripts/test-end-to-end.sh` and verify it passes

---

## Steel Thread 11: Fix --env Flag Ignored by VM and Nono Run

The `--env` flag is a persistent flag accepted by all subcommands, but `cmd_run.go` only calls `GetEnvVars()` in the container path. The VM and nono `run` paths never merge `--env` vars, so `isolarium --env FOO=bar run --type vm -- printenv FOO` silently drops the variable. The `shell` command works correctly for all types.

- [x] **Task 11.1: VM run passes --env vars to backend**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/cli/... ./internal/lima/...`
  - Observable: When `isolarium --env FOO=bar run --type vm -- printenv FOO` is executed, the `limactl shell` command includes `FOO=bar` in its environment variables.
  - Evidence: Unit test verifies that `GetEnvVars()` values are merged into the env vars map passed to the VM backend's `Exec` method in `cmd_run.go`.
  - Steps:
    - [x] Add `GetEnvVars()` merge into the VM path in `cmd_run.go` (same pattern as container path at line 221)
    - [x] Add unit test verifying VM run passes --env vars to backend

- [x] **Task 11.2: Nono run passes --env vars to backend**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/cli/... ./internal/nono/...`
  - Observable: When `isolarium --env FOO=bar run --type nono -- printenv FOO` is executed, the command's environment includes `FOO=bar`.
  - Evidence: Unit test verifies that `GetEnvVars()` values are merged into the env vars map passed to the nono backend's `Exec` method in `cmd_run.go`.
  - Steps:
    - [x] Add `GetEnvVars()` merge into the nono path in `cmd_run.go` (same pattern as container path at line 221)
    - [x] Add unit test verifying nono run passes --env vars to backend

- [x] **Task 11.3: --env flag e2e test covers VM and nono run**
  - TaskType: OUTCOME
  - Entrypoint: `./test-scripts/test-env-flag.sh`
  - Observable: The existing `test-env-flag.sh` (or an extended version) verifies `--env` works for `run` across all isolation types, not just container.
  - Evidence: `./test-scripts/test-env-flag.sh` passes with VM and nono coverage added.
  - Steps:
    - [x] Extend `test-env-flag.sh` to test `--env` with `run --type nono`
    - [x] Extend `test-env-flag.sh` to test `--env` with `run --type vm`
    - [x] Verify the script passes

---

## Change History

### 2026-03-10: Initial plan created
- 9 steel threads covering: pid.yaml parsing, ISOLARIUM_NAME/TYPE env vars, container isolation_scripts, --env flag, host scripts, VM isolation_scripts, gradlew e2e, pytest e2e, pre-commit self-test (container + VM)
- Ordered by causal dependency: parsing → container scripts → env flag → host scripts → VM scripts → e2e validation
- Steps should be implemented using TDD

### 2026-03-11 07:31 - mark-task-complete
Implemented LoadPidConfig with PidConfig/IsolationTypeConfig/ScriptEntry types. All 4 unit tests pass.

### 2026-03-11 07:41 - mark-task-complete
Added applyEnvVarDefaults in PersistentPreRunE. 7 unit tests verify precedence: explicit flag > env var > default.

### 2026-03-11 07:48 - mark-task-complete
CI workflow already runs go test ./... via test-end-to-end.sh. All 13 packages pass locally.

### 2026-03-11 07:56 - mark-task-complete
GenerateDockerfile function and 4 unit tests implemented via TDD

### 2026-03-11 08:10 - mark-task-complete
Implemented build context preparation, env var validation, and build-arg passing for container isolation_scripts

### 2026-03-11 08:27 - mark-step-complete
Created testdata/pid-yaml-test/pid.yaml with one isolation_script

### 2026-03-11 08:27 - mark-step-complete
Created testdata/pid-yaml-test/scripts/create-marker.sh

### 2026-03-11 08:27 - mark-step-complete
Created test-scripts/test-container-isolation-scripts.sh - builds, creates, verifies, destroys

### 2026-03-11 08:28 - mark-step-complete
Added test-container-isolation-scripts.sh to test-end-to-end.sh

### 2026-03-11 08:28 - mark-task-complete
E2e test passes: creates container with isolation_scripts, verifies marker file, destroys

### 2026-03-11 08:36 - mark-task-complete
Added --env persistent StringSlice flag on root command, parseEnvFlags function that splits on first = to distinguish VAR from VAR=VALUE, GetEnvVars accessor for subcommands, and 6 unit tests covering all evidence criteria

### 2026-03-11 08:46 - mark-step-complete
docker exec is constructed in internal/docker/exec.go (BuildExecCommand, BuildInteractiveExecCommand)

### 2026-03-11 08:46 - mark-step-complete
Merged GetEnvVars() into envVars map in runInContainer() and buildShellEnvVars()

### 2026-03-11 08:46 - mark-step-complete
Docker layer already adds -e KEY=VALUE flags via buildEnvFlags(); now env vars flow through from --env flag

### 2026-03-11 08:46 - mark-step-complete
Added 3 tests: ContainerPassesEnvFlagVarsToBackendExec, ContainerPassesEnvFlagVarsToBackendExecInteractive, ContainerPassesEnvFlagVarsToBackendOpenShell

### 2026-03-11 08:47 - mark-task-complete
Merged GetEnvVars() into container run and shell env var maps; verified with 3 new tests

### 2026-03-11 08:55 - mark-task-complete
All env var passing already implemented: buildEnvPrefix in exec.go, BuildExecCommand/BuildInteractiveExecCommand/BuildShellCommand all accept envVars, LimaBackend passes envVars through, CLI passes envVars to lima functions. 5 unit tests verify command construction with env vars.

### 2026-03-11 09:03 - mark-step-complete
Created test-scripts/test-env-flag.sh

### 2026-03-11 09:03 - mark-step-complete
Added to test-scripts/test-end-to-end.sh

### 2026-03-11 09:04 - mark-task-complete
test-env-flag.sh passes: creates container, runs printenv with --env TEST_VAR=hello123, asserts output

### 2026-03-11 09:13 - mark-task-complete
RunHostScripts function created with unit tests, integrated into docker backend create flow

### 2026-03-11 09:24 - mark-task-complete
Integrated RunHostScripts into LimaBackend.Create with 3 unit tests verifying host scripts run after VM create with correct env vars

### 2026-03-11 09:45 - mark-task-complete
Implemented RunVMIsolationScripts in internal/lima and integrated into LimaBackend.Create(). 11 unit tests verify command construction, env var passing, missing env var errors, and script failure propagation.

### 2026-03-11 09:53 - mark-step-complete
Read test-end-to-end-with-claude.sh to understand isolation type argument pattern

### 2026-03-11 09:53 - mark-step-complete
Modified test-end-to-end-with-gradlew.sh to accept isolation type args

### 2026-03-11 09:53 - mark-step-complete
Added Go e2e test function TestGradlewBuildInContainer_EndToEnd

### 2026-03-11 09:57 - mark-step-complete
Test creates container, runs gradlew build, asserts BUILD SUCCESSFUL - verified passing

### 2026-03-11 09:58 - mark-task-complete
Gradlew e2e test script accepts isolation type parameter, container test passes with BUILD SUCCESSFUL

### 2026-03-11 10:39 - mark-task-complete
Added container isolation type support to pytest e2e script and Go container tests

### 2026-03-11 10:56 - mark-task-complete
Created pid.yaml with 4 container isolation_scripts, created install scripts, added test verifying repo pid.yaml parses correctly

### 2026-03-11 11:28 - mark-step-complete
Added vm section to pid.yaml

### 2026-03-11 11:28 - mark-step-complete
Created scripts/vm/install-go.sh

### 2026-03-11 11:28 - mark-step-complete
Created scripts/vm/install-linters.sh

### 2026-03-11 11:28 - mark-step-complete
Created scripts/vm/install-pre-commit.sh

### 2026-03-11 11:28 - mark-step-complete
Created scripts/vm/install-codescene.sh

### 2026-03-11 11:28 - mark-step-complete
All pid.yaml parsing tests pass

### 2026-03-11 11:28 - mark-task-complete
VM section added to pid.yaml with all 4 isolation scripts and tests pass

### 2026-03-11 11:36 - mark-task-complete
Created test-precommit-in-vm.sh and added to test-end-to-end.sh conditionally

### 2026-03-11 13:15 - mark-step-complete
Updated run_test() to capture go test output and fail if it contains 'no tests to run'

### 2026-03-11 13:15 - mark-step-complete
Created e2e_gradlew_vm_test.go with TestGradlewBuildInVM_EndToEnd following the container test pattern

### 2026-03-11 13:15 - mark-step-complete
Test creates VM, runs gradlew build, and asserts BUILD SUCCESSFUL

### 2026-03-11 13:15 - mark-task-complete
Hardened run_test() with no-tests-to-run detection and created VM gradlew e2e test

### 2026-03-12 10:27 - mark-step-complete
Removed if/else conditional around test-precommit-in-container.sh in test-end-to-end.sh

### 2026-03-12 10:27 - mark-step-complete
test-precommit-in-container.sh loads .env.local via loadEnvLocalIfPresent

### 2026-03-12 10:27 - mark-step-complete
test-precommit-in-container.sh exits 0 with all hooks passing

### 2026-03-12 10:27 - mark-step-complete
test-end-to-end.sh calls test-precommit-in-container.sh unconditionally

### 2026-03-12 10:28 - mark-task-complete
Pre-commit in container test passes with all hooks including CodeScene

### 2026-03-12 10:53 - mark-step-complete
Removed if/else conditional, now calls test-precommit-in-vm.sh unconditionally

### 2026-03-12 13:43 - mark-step-complete
Added testEnvFlagWithNono function

### 2026-03-12 13:43 - mark-step-complete
Added testEnvFlagWithVM function

### 2026-03-12 13:43 - mark-step-complete
All 3 isolation types tested and passed

### 2026-03-12 13:43 - mark-task-complete
Extended test-env-flag.sh to cover container, nono, and VM; all 3 pass
