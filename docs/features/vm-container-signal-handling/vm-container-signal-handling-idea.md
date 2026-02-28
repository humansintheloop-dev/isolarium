## Problem

When pressing Ctrl-C during `isolarium run` with `--isolation-type vm` or `--isolation-type container`, the child processes (limactl shell, docker exec) may not receive the signal and shut down gracefully. This is the same architectural issue as the nono signal handling problem, applied to the vm and container backends.

## Goal

When the user presses Ctrl-C, both the `isolarium run` command and the underlying vm/container processes should receive the signal and shut down gracefully.

## Locations

TBD — details to be filled in after the nono signal handling work is completed.
