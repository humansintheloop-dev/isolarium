#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

GUARD_DIR="logs/test-all-guards"
RESUME=false

for arg in "$@"; do
    case "$arg" in
        --resume) RESUME=true ;;
        *)
            echo "Usage: $0 [--resume]"
            exit 1
            ;;
    esac
done

if [ "$RESUME" = false ]; then
    rm -rf "$GUARD_DIR"
fi
mkdir -p "$GUARD_DIR"

OVERALL_START=$(date +%s)
PASSED=()
FAILED=()
SKIPPED=()

requireNono() {
    if ! command -v nono &> /dev/null; then
        echo "FATAL: nono is not installed" >&2
        exit 1
    fi
}

requireDocker() {
    if ! docker info &> /dev/null; then
        echo "FATAL: Docker is not available" >&2
        exit 1
    fi
}

requireLima() {
    if ! command -v limactl &> /dev/null; then
        echo "FATAL: Lima is not installed" >&2
        exit 1
    fi
}

ensureClaudeTokenIsCurrent() {
    echo "--- Refreshing Claude token ---"
    claude -p Hello < /dev/null
}

timePhase() {
    local label="$1"
    shift
    local start_time
    start_time=$(date +%s)
    if "$@"; then
        local end_time
        end_time=$(date +%s)
        echo "TIMING: $label took $((end_time - start_time))s"
        PASSED+=("$label")
    else
        local end_time
        end_time=$(date +%s)
        echo "TIMING: $label FAILED after $((end_time - start_time))s"
        FAILED+=("$label")
        return 1
    fi
}

guarded() {
    local guard="$1"
    shift
    if [ -f "$GUARD_DIR/$guard" ]; then
        echo "SKIP: $guard (already done)"
        SKIPPED+=("$guard")
        return
    fi
    "$@"
    touch "$GUARD_DIR/$guard"
}

printSummary() {
    local overall_end
    overall_end=$(date +%s)
    echo ""
    echo "============================================"
    echo "  TEST SUMMARY"
    echo "============================================"
    echo "Total time: $((overall_end - OVERALL_START))s"
    echo ""
    if [ ${#PASSED[@]} -gt 0 ]; then
        echo "PASSED (${#PASSED[@]}):"
        for t in "${PASSED[@]}"; do echo "  + $t"; done
    fi
    if [ ${#SKIPPED[@]} -gt 0 ]; then
        echo ""
        echo "SKIPPED (${#SKIPPED[@]}):"
        for t in "${SKIPPED[@]}"; do echo "  ~ $t"; done
    fi
    if [ ${#FAILED[@]} -gt 0 ]; then
        echo ""
        echo "FAILED (${#FAILED[@]}):"
        for t in "${FAILED[@]}"; do echo "  ! $t"; done
    fi
    echo "============================================"
}

trap printSummary EXIT

requireNono
requireDocker
requireLima

# ── Tier 1: Unit tests ───────────────────────────────────────────

runTier1() {
    echo ""
    echo "=== Tier 1: Unit tests ==="
    echo ""

    timePhase "compile-check (all build tags)" \
        go test -run='^$' -count=1 \
        -tags=integration,integration_setup,integration_teardown,cleanup,e2e_gradlew,e2e_pytest \
        ./...

    timePhase "unit tests" go test -count=1 ./...
}

# ── Tier 2: Nono sandbox ─────────────────────────────────────────

runTier2() {
    echo ""
    echo "=== Tier 2: Nono sandbox tests ==="
    echo ""

    timePhase "nono integration" go test -tags=integration ./internal/nono/...

    timePhase "e2e gradlew (nono)" "$SCRIPT_DIR/test-end-to-end-with-gradlew.sh" --force nono

    timePhase "e2e pytest (nono)" "$SCRIPT_DIR/test-end-to-end-with-pytest.sh" --force nono

    timePhase "precommit in nono" "$SCRIPT_DIR/test-precommit-in-nono.sh"
}

# ── Tier 3: Docker container tests ───────────────────────────────

runTier3() {
    echo ""
    echo "=== Tier 3: Docker container tests ==="
    echo ""

    timePhase "docker integration (go tests)" \
        go test -tags=integration ./internal/docker/...

    timePhase "container isolation scripts" \
        "$SCRIPT_DIR/test-container-isolation-scripts.sh"

    timePhase "host scripts" "$SCRIPT_DIR/test-host-scripts.sh"

    timePhase "e2e gradlew (container)" \
        "$SCRIPT_DIR/test-end-to-end-with-gradlew.sh" --force container

    timePhase "e2e pytest (container)" \
        "$SCRIPT_DIR/test-end-to-end-with-pytest.sh" --force container

    timePhase "precommit in container" \
        "$SCRIPT_DIR/test-precommit-in-container.sh"
}

# ── Tier 4: Lima VM tests ────────────────────────────────────────

runTier4() {
    echo ""
    echo "=== Tier 4: Lima VM tests ==="
    echo ""

    guarded "lima-integration" timePhase "lima integration" "$SCRIPT_DIR/test-lima-integration.sh"
    guarded "security-tests" timePhase "security tests" "$SCRIPT_DIR/test-security.sh"
    guarded "claude-integration" timePhase "claude integration" "$SCRIPT_DIR/test-claude-integration.sh"
    guarded "precommit-in-vm" timePhase "precommit in VM" "$SCRIPT_DIR/test-precommit-in-vm.sh"
    guarded "e2e-gradlew-vm" timePhase "e2e gradlew (vm)" "$SCRIPT_DIR/test-end-to-end-with-gradlew.sh" --force vm
    guarded "e2e-pytest-vm" timePhase "e2e pytest (vm)" "$SCRIPT_DIR/test-end-to-end-with-pytest.sh" --force vm
    guarded "cleanup" timePhase "cleanup" "$SCRIPT_DIR/test-cleanup.sh"
}

# ── Tier 5: E2E Claude tests ─────────────────────────────────────

runClaudeNono() {
    ensureClaudeTokenIsCurrent
    timePhase "e2e claude (nono)" "$SCRIPT_DIR/test-end-to-end-with-claude.sh" --force nono
}

runClaudeContainer() {
    ensureClaudeTokenIsCurrent
    timePhase "e2e claude (container)" "$SCRIPT_DIR/test-end-to-end-with-claude.sh" --force container
}

runClaudeVM() {
    ensureClaudeTokenIsCurrent
    timePhase "e2e claude (vm)" "$SCRIPT_DIR/test-end-to-end-with-claude.sh" --force vm
}

runTier5() {
    echo ""
    echo "=== Tier 5: E2E Claude tests ==="
    echo ""

    runClaudeNono
    runClaudeContainer
    guarded "e2e-claude-vm" runClaudeVM
}

# ── Run all tiers ─────────────────────────────────────────────────

guarded "tier1" runTier1
guarded "tier2" runTier2
guarded "tier3" runTier3
guarded "tier4" runTier4
guarded "tier5" runTier5

if [ ${#FAILED[@]} -gt 0 ]; then
    exit 1
fi
