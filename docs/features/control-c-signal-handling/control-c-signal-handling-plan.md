Now I have a thorough understanding of the codebase. Let me generate the plan.

---

# Ctrl-C Signal Handling for Nono Backend — Implementation Plan

**Plan file:** `docs/features/control-c-signal-handling/control-c-signal-handling-plan.md`

## Idea Type: A (User-facing feature)

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

This plan adds signal handling to `internal/nono/exec.go:runWithCommand` so that Ctrl-C (SIGINT) and SIGTERM are forwarded to the nono child process group, with a grace period and force-kill escalation. The implementation is scoped to the nono backend only.

**Existing infrastructure (no changes needed):**
- CI: `.github/workflows/ci.yml` runs `test-scripts/test-end-to-end.sh`
- Unit tests: `test-scripts/test-unit.sh` runs `go test ./...`
- Nono integration tests: `test-scripts/test-nono-integration.sh`

New Go unit tests in `internal/nono/exec_test.go` will be automatically validated by the existing CI pipeline through `go test ./...`.

**Key design decision for testability:** Extract a `runWithSignals(cmdArgs, env, sigCh, gracePeriod)` function from `runWithCommand`. The public function sets up `signal.Notify` and calls `runWithSignals` with a 10-second grace period. Tests call `runWithSignals` directly with a test-controlled signal channel and short grace period, avoiding process-level signal interference.

All tasks should be implemented using TDD.

---

## Steel Thread 1: Graceful SIGINT Shutdown

Implements Scenario 1 from the spec: user presses Ctrl-C, isolarium forwards SIGINT to the nono process group, and exits with code 130. Also establishes backward compatibility (Scenario 5).

- [ ] **Task 1.1: runWithCommand starts nono in its own process group and preserves normal exit behavior**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/nono/...`
  - Observable: `runWithCommand` starts child process in its own process group (via `Setpgid`); normal completion returns the child's exit code unchanged (0 for success, non-zero propagated)
  - Evidence: Unit tests in `internal/nono/exec_test.go` verify exit code 0 for successful command and non-zero exit code propagation; `go test ./internal/nono/...` passes
  - Steps:
    - [ ] Create `internal/nono/exec_test.go` with test that calls `runWithSignals` with `["echo", "hello"]` and asserts exit code 0
    - [ ] Create test that calls `runWithSignals` with `["sh", "-c", "exit 42"]` and asserts exit code 42
    - [ ] Extract `runWithSignals(cmdArgs []string, envVars map[string]string, sigCh <-chan os.Signal, gracePeriod time.Duration) (int, error)` from `runWithCommand` in `internal/nono/exec.go`
    - [ ] Set `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` on the child command
    - [ ] Replace `cmd.Run()` with `cmd.Start()` + `cmd.Wait()` in the extracted function
    - [ ] Have `runWithCommand` create a signal channel with `signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)`, then delegate to `runWithSignals` with a 10-second grace period
    - [ ] Verify both tests pass

- [ ] **Task 1.2: runWithCommand forwards SIGINT to nono process group and exits with code 130**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/nono/...`
  - Observable: When SIGINT is received while child is running, SIGINT is forwarded to the child process group via `syscall.Kill(-pgid, SIGINT)`; function returns exit code 130 (128 + 2)
  - Evidence: Unit test sends SIGINT via test signal channel while child runs `sleep`, verifies child terminates and function returns exit code 130; `go test ./internal/nono/...` passes
  - Steps:
    - [ ] Write test: call `runWithSignals` with `["sleep", "100"]`, send `syscall.SIGINT` on the test signal channel after 200ms, assert exit code is 130
    - [ ] Add signal listener goroutine in `runWithSignals`: select on sigCh and the process done channel
    - [ ] When signal received, forward to child process group via `syscall.Kill(-cmd.Process.Pid, sig)` (negative PID targets the group)
    - [ ] Compute exit code as 128 + signal number (SIGINT=2 → 130)
    - [ ] Verify all tests pass (including backward-compat tests from Task 1.1)

## Steel Thread 2: SIGTERM Forwarding

Implements Scenario 4 from the spec: CI sends SIGTERM to isolarium, isolarium forwards SIGTERM to the nono process group, and exits with code 143.

- [ ] **Task 2.1: runWithCommand forwards SIGTERM to nono process group and exits with code 143**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/nono/...`
  - Observable: When SIGTERM is received while child is running, SIGTERM is forwarded to the child process group; function returns exit code 143 (128 + 15)
  - Evidence: Unit test sends SIGTERM via test signal channel, verifies exit code is 143; `go test ./internal/nono/...` passes
  - Steps:
    - [ ] Write test: call `runWithSignals` with `["sleep", "100"]`, send `syscall.SIGTERM` on the test signal channel after 200ms, assert exit code is 143
    - [ ] Verify the signal forwarding implementation from Steel Thread 1 handles SIGTERM correctly (the generic signal number calculation `128 + sig` should already cover this)
    - [ ] Verify all tests pass

## Steel Thread 3: Timeout Escalation to SIGKILL

Implements Scenario 2 from the spec: child process ignores the forwarded signal, isolarium waits the grace period, then sends SIGKILL to the process group.

- [ ] **Task 3.1: runWithCommand sends SIGKILL to process group after grace period when child ignores signal**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/nono/...`
  - Observable: When child process doesn't exit within grace period after signal forwarding, SIGKILL is sent to the process group; function returns the signal-based exit code (130 for SIGINT)
  - Evidence: Unit test uses a signal-ignoring child process (`sh -c 'trap "" INT; sleep 100'`) and a 1-second grace period; sends SIGINT, verifies process is killed and exit code is 130; `go test ./internal/nono/...` passes
  - Steps:
    - [ ] Write test: call `runWithSignals` with `["sh", "-c", "trap \"\" INT; sleep 100"]`, 1-second grace period, send SIGINT on test channel, assert process terminates within ~2 seconds and exit code is 130
    - [ ] Add grace period timer in `runWithSignals`: after forwarding signal, start a `time.After(gracePeriod)` timer
    - [ ] If timer fires before child exits, send `syscall.Kill(-pgid, syscall.SIGKILL)` to the process group
    - [ ] Verify all tests pass

## Steel Thread 4: Double Signal Force Kill

Implements Scenario 3 from the spec: user presses Ctrl-C a second time during the grace period, isolarium immediately SIGKILLs the process group.

- [ ] **Task 4.1: Second signal during grace period immediately SIGKILLs process group**
  - TaskType: OUTCOME
  - Entrypoint: `go test ./internal/nono/...`
  - Observable: When a second SIGINT or SIGTERM arrives during the grace period, SIGKILL is sent immediately to the process group without waiting for the grace period to expire; function returns the signal-based exit code (130 for SIGINT)
  - Evidence: Unit test uses a signal-ignoring child process with 30-second grace period; sends SIGINT, then sends second SIGINT 200ms later; verifies process terminates within ~1 second (well before the 30-second grace period); `go test ./internal/nono/...` passes
  - Steps:
    - [ ] Write test: call `runWithSignals` with `["sh", "-c", "trap \"\" INT; sleep 100"]`, 30-second grace period, send SIGINT, send second SIGINT 200ms later, assert process terminates within ~1 second and exit code is 130
    - [ ] Modify the signal listener in `runWithSignals`: after first signal and during grace period, continue listening on sigCh; on second signal, immediately send `syscall.Kill(-pgid, syscall.SIGKILL)`
    - [ ] Verify all tests pass
