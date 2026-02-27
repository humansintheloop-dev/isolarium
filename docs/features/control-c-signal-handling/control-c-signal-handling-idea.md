## Problem

When pressing Ctrl-C during `isolarium run --isolation-type nono`, the isolarium process exits but the nono process (and its child process tree) continues running in the background. The user must manually find and kill the orphaned processes.

This happens because `runWithCommand` in `internal/nono/exec.go` uses plain `exec.Command(...).Run()` with no process group management or signal forwarding.

## Goal

When the user presses Ctrl-C (or the process receives SIGTERM), isolarium should forward the signal to the nono process group and shut down gracefully. If nono doesn't exit within 10 seconds, SIGKILL the process group. A second Ctrl-C should immediately SIGKILL and exit.

## Behavior Summary

- Forward both SIGINT and SIGTERM to the nono process group
- 10-second grace period, then SIGKILL
- Second Ctrl-C = immediate SIGKILL
- Exit code 130 (SIGINT) or 143 (SIGTERM)

## Locations

- **Definition** — `internal/nono/exec.go:17` (`runWithCommand`) — where the nono process is spawned
- **Call sites** — `internal/nono/exec.go:9` (`ExecCommand`), `internal/nono/exec.go:13` (`ExecInteractiveCommand`)
- **Existing test** — `cmd/isolarium/main_test.go:394` (`TestRunCommand_TerminatesOnSIGINT`)
- **Unchanged** — vm and container backends (separate idea: `docs/features/vm-container-signal-handling/`)
