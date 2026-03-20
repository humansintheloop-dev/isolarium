//go:build e2e_pytest

package main

import (
	"strings"
	"testing"
)

func TestPytestInVM_EndToEnd(t *testing.T) {
	binary := buildPytestBinary(t)
	projectRoot := pytestProjectRoot(t)

	repo := copyToTempGitRepo(t, projectRoot+"/testdata/python-cli-app")

	createVMFromTempRepo(t, binary, repo, projectRoot)
	t.Cleanup(func() { destroyVM(t, binary, repo) })

	output, err := runInTempVM(t, binary, repo, "bash", "-c", "export PATH=$HOME/.local/bin:$PATH && rm -rf .venv && uv run pytest -v")
	t.Logf("pytest output:\n%s", output)
	if err != nil {
		t.Fatalf("pytest in VM failed: %v", err)
	}
	if !strings.Contains(string(output), "2 passed") {
		t.Fatal("expected 2 passed in output")
	}
}

func TestGreeterCliInVM_EndToEnd(t *testing.T) {
	binary := buildPytestBinary(t)
	projectRoot := pytestProjectRoot(t)

	repo := copyToTempGitRepo(t, projectRoot+"/testdata/python-cli-app")

	createVMFromTempRepo(t, binary, repo, projectRoot)
	t.Cleanup(func() { destroyVM(t, binary, repo) })

	output, err := runInTempVM(t, binary, repo, "bash", "-c", "export PATH=$HOME/.local/bin:$PATH && rm -rf .venv && uv run greeter VM")
	t.Logf("greeter output:\n%s", output)
	if err != nil {
		t.Fatalf("greeter CLI in VM failed: %v", err)
	}
	if !strings.Contains(string(output), "Hello, VM!") {
		t.Fatal("expected 'Hello, VM!' in output")
	}
}
