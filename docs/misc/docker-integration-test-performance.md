# Docker Integration Test Performance Analysis

`test-scripts/test-docker-integration.sh` runs 5 phases sequentially.

## Measured Phase Timings

Three runs of the full suite:

| Phase | Script | Run 1 | Run 3 |
|---|---|---|---|
| 1 | `go test -tags=integration` | 0s | 1s |
| 2 | `test-container-isolation-scripts.sh` | 7s | 3s |
| 3 | **`test-env-flag.sh`** | **239s** | **234s** |
| 4 | `test-host-scripts.sh` | 7s | 4s |
| 5 | `test-precommit-in-container.sh` | 129s | 28s |
| | **Total** | **383s** | **270s** |

(Run 2 only had step-level timing for the precommit script, not phase-level totals.)

### `test-precommit-in-container.sh` step-level breakdown (runs 2 and 3)

| Step | Run 2 | Run 3 |
|---|---|---|
| `go build` | 0s | 1s |
| `isolarium create` | 4s | 2s |
| `cs check` | 2s | 1s |
| `pre-commit run --all-files` | 23s | 23s |
| `isolarium destroy` | 0s | 1s |
| **Total** | **29s** | **28s** |

## Key Findings

### `test-env-flag.sh` — ~235s (consistently 85-90% of total)

This script tests the `--env` flag across three isolation types (container, nono, VM). The **VM test dominates**: creating a Lima VM just to run `printenv` accounts for nearly all the time. The container and nono portions are fast.

### `test-precommit-in-container.sh` — variable (28s to 129s)

Step-level timing from runs 2 and 3 shows this script consistently takes ~28s, with `pre-commit run --all-files` (23s) as the dominant step. Run 1 took 129s but had no step-level timing, so the cause of the difference is unknown.

### Everything else — ~8s (consistent)

Phases 1, 2, and 4 are fast and not worth optimizing.

## `test-env-flag.sh` coverage analysis

`test-env-flag.sh` is an end-to-end test: it builds the `isolarium` binary, runs it with `--env TEST_VAR=hello123`, and verifies `printenv` sees the value inside the isolation environment.

The flow it tests (CLI flag -> cmd_run wiring -> backend exec -> `printenv`) is covered at every intermediate step by unit tests:

| Step in the flow | Unit test | Location |
|---|---|---|
| CLI `--env` flag parsing | `TestParseEnvFlags_*` | `internal/cli/env_flag_test.go` |
| Flag values reach container backend | `TestRunCommand_ContainerPassesEnvFlagVarsToBackendExec` | `internal/cli/cmd_run_test.go:362` |
| Flag values reach VM backend | `TestRunCommand_VMPassesEnvFlagVarsToBackend` | `internal/cli/cmd_run_test.go:381` |
| Flag values reach nono backend | `TestRunCommand_NonoPassesEnvFlagVarsToBackend` | `internal/cli/cmd_run_test.go:401` |
| `BuildExecCommand` constructs correct args | `TestBuildExecCommand_WithEnvVars` | `internal/docker/exec_test.go:15`, `internal/lima/exec_test.go:52` |

The final step (actual exec with real infrastructure) is covered by integration tests:

| Integration test | Location |
|---|---|
| `TestExecCommand_WithEnvVars_Integration` (VM) | `internal/lima/integration_test.go:106`, run by `test-lima-integration.sh` |

The only gap `test-env-flag.sh` fills is a true end-to-end test exercising the built binary through real infrastructure. Every intermediate step is already tested independently.

## Suggested Optimizations

### High impact: remove `test-env-flag.sh` from this script (saves ~235s)

The end-to-end value of `test-env-flag.sh` is narrow given the existing unit and integration test coverage. The VM portion (234s) is the most wasteful — it creates and destroys a full VM for a single `printenv` check. Removing this call would cut the suite from ~270s to ~36s.

If end-to-end coverage is still desired, the container test alone (a few seconds) provides that confidence without the VM cost.

### Lower impact

1. **`pre-commit run --all-files`** takes 23s — this is the actual work being tested, so likely irreducible.
2. **Phases 2 and 4** take ~3-7s each — not worth optimizing.

## After removing `test-env-flag.sh`

After removing `test-env-flag.sh` from `test-docker-integration.sh`, the suite runs 4 phases:

| Phase | Script | Time |
|---|---|---|
| 1 | `go test -tags=integration` | 1s |
| 2 | `test-container-isolation-scripts.sh` | 5s |
| 3 | `test-host-scripts.sh` | 2s |
| 4 | `test-precommit-in-container.sh` | 29s |
| | **Total** | **37s** |

### `test-precommit-in-container.sh` step-level breakdown

| Step | Time |
|---|---|
| `go build` | 0s |
| `isolarium create` | 2s |
| `cs check` (verify CodeScene works in container) | 1s |
| make a harmless file change in container | 0s |
| `pre-commit run --all-files` (breakdown below) | 25s |
| `isolarium destroy` | 0s |
| **Total** | **29s** |

#### `pre-commit run --all-files` per-hook breakdown

| Hook | Time |
|---|---|
| `gitleaks` | 10s |
| `shellcheck` | 5s |
| `go-vet` | 4s |
| `precommit-check` | 4s |
| `run-codescene` | 1s |
| `golangci-lint` | 1s |
| **Total** | **25s** |

Using `--files` instead of `--all-files` with a minimal set of files (one `.go`, one `.sh`) was also measured — no meaningful difference. `gitleaks` still takes 10s because it scans git history, not just the specified files. The other `pass_filenames: false` hooks also run their full commands regardless of `--files`.

Total time reduced from ~270s to 37s (86% reduction). `pre-commit run --all-files` (24s) now accounts for most of the remaining time.
