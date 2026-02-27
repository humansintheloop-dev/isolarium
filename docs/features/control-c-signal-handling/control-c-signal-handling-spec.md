# Ctrl-C Signal Handling for Nono Backend — Specification

## Purpose and Background

`isolarium run --isolation-type nono` spawns a `nono run` child process that sandboxes a user command on the host machine. Currently, when the user presses Ctrl-C, isolarium exits but the nono process tree continues running in the background. Users must manually find and kill orphaned processes.

The root cause is in `internal/nono/exec.go:runWithCommand`, which uses `exec.Command(...).Run()` with no process group management or signal forwarding.

## Target Users

- **Developer** — runs `isolarium run --type nono -- <cmd>` interactively and expects Ctrl-C to cleanly stop everything.
- **CI/automation system** — sends SIGTERM to isolarium and expects the entire process tree to shut down.

## Problem Statement

When isolarium receives SIGINT (Ctrl-C) or SIGTERM, the nono child process and its descendants are orphaned. The user has no indication that processes are still running and must resort to `ps` / `kill` to clean up.

## Goals

1. Ctrl-C (SIGINT) and SIGTERM are forwarded to the nono process group.
2. Nono and its child processes shut down cleanly.
3. If the nono process doesn't exit within a grace period, it is forcibly killed.
4. A second Ctrl-C immediately force-kills the process group.

## In Scope

- Signal handling for the **nono backend only** (`internal/nono/exec.go`)
- Both `ExecCommand` and `ExecInteractiveCommand` (they share `runWithCommand`)
- SIGINT and SIGTERM forwarding
- Graceful shutdown with timeout escalation to SIGKILL
- Correct exit codes (130 for SIGINT, 143 for SIGTERM)

## Out of Scope

- VM backend (`internal/lima/`) — tracked in `docs/features/vm-container-signal-handling/`
- Container backend (`internal/docker/`) — tracked in `docs/features/vm-container-signal-handling/`
- Other signals (SIGHUP, SIGQUIT, etc.)
- Changes to the `nono` binary itself

## Functional Requirements

### FR-1: Process group isolation

The nono child process must be started in its own process group (`SysProcAttr.Setpgid = true`) so that signals can be sent to the entire group.

### FR-2: Signal interception

Isolarium must intercept SIGINT and SIGTERM using `signal.Notify` before starting the nono process.

### FR-3: Signal forwarding

When isolarium receives SIGINT or SIGTERM, it must forward the same signal to the nono process group via `syscall.Kill(-pgid, signal)`.

### FR-4: Graceful shutdown with timeout

After forwarding the signal, isolarium must wait up to 10 seconds for the nono process to exit. If it hasn't exited after 10 seconds, isolarium must send SIGKILL to the process group.

### FR-5: Force kill on second signal

If isolarium receives a second SIGINT or SIGTERM during the grace period, it must immediately send SIGKILL to the nono process group and exit.

### FR-6: Exit codes

- SIGINT termination: exit code **130** (128 + 2)
- SIGTERM termination: exit code **143** (128 + 15)
- Normal completion: the nono process's exit code (unchanged from current behavior)

### FR-7: Backward compatibility

When no signal is received, behavior must be identical to today — `runWithCommand` returns the nono process's exit code or an error.

## Security Requirements

No new security surface. Signal handling is process-local and does not introduce new endpoints, permissions, or authorization checks. The nono sandbox permissions are unchanged.

## Non-Functional Requirements

### Reliability

- Signal handling must not introduce race conditions between signal delivery and process exit.
- The SIGKILL fallback ensures isolarium never hangs indefinitely.

### Performance

- No measurable overhead on the happy path (no signal received).

### Compatibility

- macOS only (nono is a macOS sandbox tool). No Linux/Windows support required.
- Must work with the existing `nono` binary — no changes to nono itself.

## Success Metrics

1. `isolarium run --type nono -- sleep 3600` + Ctrl-C: both isolarium and nono exit within 1 second. No orphaned processes.
2. `isolarium run --type nono -- <long-running-cmd>` + SIGTERM: same clean shutdown behavior.
3. Existing test `TestRunCommand_TerminatesOnSIGINT` continues to pass.
4. No orphaned `nono` processes after any signal-based shutdown.

## User Stories

### US-1: Clean Ctrl-C shutdown

As a developer running `isolarium run --type nono -- <cmd>`, when I press Ctrl-C, I expect all processes (isolarium, nono, and the sandboxed command) to stop so that I don't have orphaned processes consuming resources.

### US-2: Force kill on stuck process

As a developer, when I press Ctrl-C and the sandboxed command doesn't stop within a reasonable time, I expect isolarium to force-kill it so I'm not left waiting indefinitely.

### US-3: Immediate abort

As a developer, when I press Ctrl-C a second time during shutdown, I expect everything to stop immediately so I can regain control of my terminal.

### US-4: CI/automation graceful shutdown

As a CI system, when I send SIGTERM to isolarium, I expect the nono process tree to shut down cleanly within 10 seconds so that the CI job can proceed to cleanup.

## Scenarios

### Scenario 1: Graceful Ctrl-C shutdown (primary end-to-end scenario)

1. User runs `isolarium run --type nono -- sleep 3600`
2. Nono process starts in its own process group
3. User presses Ctrl-C
4. Isolarium receives SIGINT, forwards SIGINT to the nono process group
5. Nono and sleep exit
6. Isolarium exits with code 130

### Scenario 2: Timeout escalation to SIGKILL

1. User runs `isolarium run --type nono -- <signal-ignoring-cmd>`
2. User presses Ctrl-C
3. Isolarium forwards SIGINT to the nono process group
4. 10 seconds pass, nono is still running
5. Isolarium sends SIGKILL to the process group
6. Isolarium exits with code 130

### Scenario 3: Double Ctrl-C force kill

1. User runs `isolarium run --type nono -- <cmd>`
2. User presses Ctrl-C
3. Isolarium forwards SIGINT, begins waiting
4. User presses Ctrl-C again within the grace period
5. Isolarium immediately sends SIGKILL to the process group
6. Isolarium exits with code 130

### Scenario 4: SIGTERM from CI

1. CI system runs `isolarium run --type nono -- <build-cmd>`
2. CI sends SIGTERM to isolarium
3. Isolarium forwards SIGTERM to the nono process group
4. Nono and the build command exit
5. Isolarium exits with code 143

### Scenario 5: Normal completion (no signal)

1. User runs `isolarium run --type nono -- echo hello`
2. Nono executes `echo hello` and exits with code 0
3. Isolarium exits with code 0
4. No difference from current behavior

## Affected Code Locations

| Role | Location |
|------|----------|
| **Definition** | `internal/nono/exec.go:17` — `runWithCommand` |
| **Call sites** | `internal/nono/exec.go:9` — `ExecCommand` |
| | `internal/nono/exec.go:13` — `ExecInteractiveCommand` |
| **Existing test** | `cmd/isolarium/main_test.go:394` — `TestRunCommand_TerminatesOnSIGINT` |
| **Unchanged** | `internal/lima/` — vm backend |
| **Unchanged** | `internal/docker/` — container backend |
