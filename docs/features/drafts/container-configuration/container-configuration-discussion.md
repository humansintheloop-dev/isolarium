# Container Configuration - Discussion

## Classification

**A. User-facing feature**

Rationale: `pid.yaml` is a configuration mechanism that project authors use directly to define how their isolated environments are set up. It extends the CLI's `create` command with new behavior, adds `--env` flags, and introduces `ISOLARIUM_NAME`/`ISOLARIUM_TYPE` env vars. It's not an architecture POC (the isolation backends already exist), not pure infrastructure (it's project-author-facing), and not educational.

## Questions and Answers

### Q1: Are the e2e test script changes and pid.yaml one feature, or is extending the test scripts the motivation that exposed the need for pid.yaml?

**A1:** Yes, they are one feature. The core deliverable is the `pid.yaml` configuration mechanism. Extending the test scripts to support multiple isolation types is a concrete use case that drives and validates it.

### Q2: When does pid.yaml get processed - at create time, run/shell time, or both?

**A2:** Option A - both `host_scripts` and `isolation_scripts` run during `isolarium create`. Everything is set up at creation time.

### Q3: Should container isolation_scripts be baked into the Dockerfile or run via docker exec after start?

**A3:** Option A - Baked into the Dockerfile as `RUN` steps. This benefits from Docker layer caching. Environment variables will be passed as `--build-arg`.

### Q4: How do isolation_scripts relate to the embedded Dockerfile?

**A4:** Option A - The embedded Dockerfile remains the base. Isolation_scripts from pid.yaml are appended as additional `RUN` layers at the end. Projects do not provide their own Dockerfile.

### Q5: When do host_scripts run relative to environment creation?

**A5:** Option A - Host scripts run after the environment is created, so they can use `isolarium shell` to interact with it. There is no host-side installation step; host_scripts exist to orchestrate actions that require host access (e.g., secrets) and push them into the environment.

### Q6: Should processing pid.yaml be part of `isolarium create` or a separate command?

**A6:** Option A - Part of `isolarium create`. Create does everything in one shot: build image (with isolation_scripts baked in), start container, run host_scripts.

### Q7: Where does pid.yaml live?

**A7:** Option A - In the project repo root (the directory mounted into the container). Each project defines its own isolation config.

### Q8: How do isolation_scripts get into the Docker image at build time?

**A8:** Option A - Copy the referenced scripts into the temp build context directory alongside the generated Dockerfile before building. Simple and self-contained.

### Q9: Where does the --env flag live?

**A9:** Option B - A persistent flag on the root command, available to all subcommands. This means `--env` vars can be used as build-args during `create`, and as runtime env vars during `run`/`shell`.

### Q10: Should VM support for pid.yaml be in scope or deferred?

**A10:** Option B - Both containers and VMs are in scope. For VMs, isolation_scripts would run via limactl shell/SSH rather than being baked into a Dockerfile.

### Q11: For VMs, isolation_scripts run via limactl shell after VM creation, then host_scripts run. Correct?

**A11:** Yes. VM create sequence: 1) Create/start VM, 2) Run isolation_scripts via limactl shell, 3) Run host_scripts.

### Q12: Should the steel thread use existing testdata projects or create new ones?

**A12:** Option A - Add `pid.yaml` to existing testdata projects (e.g. `spring-boot-app`).

### Q13: How are env vars handled for build-time vs runtime?

**A13:** The caller is responsible for passing what's needed. All `--env` vars provided to `create` become `--build-arg` for the Docker build. All `--env` vars provided to `run`/`shell` become runtime env vars. No automatic coupling between the two - the caller passes what's required for each command.

### Q14: How do host_scripts know the environment name and type?

**A14:** Option A - Isolarium sets `ISOLARIUM_NAME` and `ISOLARIUM_TYPE` as environment variables before invoking host_scripts. Since isolarium reads these env vars itself, host_scripts can simply call `isolarium shell ...` without explicitly passing `--name`/`--type` flags - they flow through automatically.

### Q15: Are --env vars available to host_scripts?

**A15:** Host_scripts inherit the full process environment, which includes any `--env` vars. They don't need special handling - the env vars are just present in the environment when the script runs.

### Q16: Does spring-boot-app need isolation_scripts for container mode?

**A16:** Option A - The base image is sufficient. Spring-boot-app's pid.yaml would be empty/minimal since Java 17 and Gradle are already in the base Dockerfile.

### Q17: What validates the pid.yaml isolation_scripts machinery in the steel thread?

**A17:** A test case that runs isolarium on its own repo: create an isolated environment, make a harmless file change, and trigger pre-commit hooks. This is a real use case because the pre-commit hooks require tools (shellcheck, golangci-lint, go, codescene CLI) that aren't in the base container image - so isolation_scripts must install them.

### Q18: Should the pre-commit test case include codescene (which requires secrets)?

**A18:** Option B - Include codescene, passing tokens via `--env`. This is a full integration test that requires secrets.

### Q19: Does pid.yaml support different script lists per isolation type?

**A19:** Yes. The schema is keyed by isolation type (`container`, `vm`), each with its own `host_scripts` and `isolation_scripts`. This allows different install methods per environment (e.g. `apt-get` in containers vs `brew` in macOS VMs).

### Q20: What happens if pid.yaml scripts fail during create?

**A20:** Option B - Fail hard, but leave the environment in place for debugging. The create command returns an error but does not clean up the container/VM.

### Q21: Behavior when pid.yaml is absent or has no entry for current --type?

**A21:** Yes - `isolarium create` behaves exactly as it does today. pid.yaml is fully optional and backward-compatible.

### Derived: Pre-commit tool requirements

From `.pre-commit-config.yaml`: remote repo hooks (shellcheck, gitleaks) are auto-managed by `pre-commit`. Local hooks require pre-installed tools: `go`, `golangci-lint`, `codescene CLI` (`run-codescene.sh`), and `pre-commit` itself. These would be installed by isolation_scripts in the isolarium project's own `pid.yaml`.

### Q22: pid.yaml scripts should declare their required env vars instead of requiring --env on the command line

**A22:** Yes. Requiring `--env CS_ACCESS_TOKEN --env CS_ACE_ACCESS_TOKEN` on every `create` invocation is tedious and error-prone. Instead, each script entry in pid.yaml declares the env vars it needs. Isolarium reads values from the process environment (or `.env.local`) automatically during create. The `--env` flag remains for ad-hoc use on `run`/`shell` but is no longer the primary mechanism for create-time env vars.

This changes the pid.yaml schema: script entries can be either a simple string (path only) or an object with `path` and `env` fields.

### Derived: ISOLARIUM_NAME/ISOLARIUM_TYPE env var support

Currently isolarium does not read `ISOLARIUM_NAME`/`ISOLARIUM_TYPE` from the environment. This would be new behavior: these env vars become defaults for `--name`/`--type` flags. This enables host_scripts to call `isolarium shell` without repeating flags.

