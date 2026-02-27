# Control-C Signal Handling - Discussion

## Classification

**Type:** A. User-facing feature (bug fix)

**Rationale:** This directly affects user experience — when a user presses Ctrl-C during `isolarium run`, they expect all processes to stop. Currently, orphaned nono processes remain running, requiring manual cleanup.

## Codebase Analysis (pre-discussion)

- All three backends (vm, nono, container) use `exec.Command(...).Run()` with no signal forwarding
- No signal handling exists in production code (`os/signal`, `signal.Notify`, `syscall.SIG*` are absent)
- Existing test `TestRunCommand_TerminatesOnSIGINT` verifies isolarium terminates on SIGINT within 10s
- The nono backend spawns `nono run ... -- <user-cmd>`, which itself creates a child process tree

## Questions and Answers

### Q1: Scope — all backends or just nono?

The idea file mentions only `--isolation-type nono`, but the same architectural pattern (no signal forwarding) exists in all three backends (vm, nono, container). Should we:

A. Fix only nono (as described in the idea file)
B. Fix all three backends (vm, nono, container) for consistent Ctrl-C behavior
C. Fix nono now, with a follow-up for the others

**Answer:** A. Fix only nono. A separate idea was created at `docs/features/vm-container-signal-handling/` for the other two backends.

### Q2: Signal forwarding strategy

Looking at `internal/nono/exec.go:17-32`, the current `runWithCommand` does `exec.Command(...).Run()` with no process group or signal handling. There are two main approaches to ensure the nono child process receives SIGINT:

A. **Process group** — Start the nono process in its own process group (`Setpgid: true`), then trap SIGINT in isolarium and forward it to the entire process group via `syscall.Kill(-pgid, syscall.SIGINT)`
B. **syscall.Exec (exec replacement)** — Replace the isolarium process entirely with the nono process using `syscall.Exec`. Ctrl-C then goes directly to nono since it *is* the process. Isolarium ceases to exist after the exec call.
C. **Context-based cancellation** — Use `exec.CommandContext` with a signal-triggered context cancellation to kill the child process

Do you have a preference, or should I choose a sensible default?

**Answer:** A. Process group — trap SIGINT in isolarium and forward to the nono process group.

### Q3: Graceful shutdown timeout

After forwarding SIGINT to the nono process group, should isolarium:

A. **Wait indefinitely** for nono to exit (simple, but risks hanging if nono ignores SIGINT)
B. **Wait with timeout, then SIGKILL** — e.g., wait 5 seconds after SIGINT, then send SIGKILL to the process group if still running
C. **Forward and exit immediately** — send SIGINT to the group and exit without waiting

Default assumption: B with a 10-second timeout (matches the existing test expectation in `TestRunCommand_TerminatesOnSIGINT`).

**Answer:** B. Wait with timeout (10 seconds), then SIGKILL.

### Q4: Multiple signals (second Ctrl-C)

If the user presses Ctrl-C a second time while waiting for the graceful shutdown, should isolarium:

A. **Immediately SIGKILL** the process group and exit (common UX pattern — "press Ctrl-C again to force quit")
B. **Ignore** subsequent SIGINTs during the grace period

Default assumption: A — this is the standard user expectation.

**Answer:** A. Second Ctrl-C immediately SIGKILLs the process group and exits.

### Q5: Exit code behavior

When isolarium shuts down due to Ctrl-C, what exit code should it return?

A. **130** — the Unix convention for processes killed by SIGINT (128 + signal number 2)
B. **The nono process's exit code** — whatever nono returned after receiving the signal
C. **1** — generic error

Default assumption: A — 130 is the standard convention and allows callers to distinguish signal termination from other errors.

**Answer:** A. Exit code 130 (standard SIGINT convention).

### Q6: Handle SIGTERM too?

In CI/automation contexts, processes typically receive SIGTERM (not SIGINT) for graceful shutdown. Should isolarium also forward SIGTERM to the nono process group using the same logic (forward, wait 10s, SIGKILL)?

A. **Yes** — handle both SIGINT and SIGTERM with the same forwarding logic
B. **No** — only handle SIGINT (Ctrl-C); SIGTERM is out of scope for now

Default assumption: A — it's minimal extra work and makes isolarium well-behaved in automated environments.

**Answer:** A. Handle both SIGINT and SIGTERM with the same forwarding logic.

## Summary of Decisions

| Decision | Choice |
|----------|--------|
| Scope | Nono backend only |
| Strategy | Process group (Setpgid + forward to -pgid) |
| Grace period | 10 seconds, then SIGKILL |
| Second Ctrl-C | Immediate SIGKILL |
| Exit code | 130 (SIGINT) / 143 (SIGTERM) |
| Signals handled | Both SIGINT and SIGTERM |
| Applies to | Both ExecCommand and ExecInteractiveCommand (shared runWithCommand) |
