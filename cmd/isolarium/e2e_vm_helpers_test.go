//go:build e2e_gradlew || e2e_pytest || e2e_claude

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func e2eTestOrg() string {
	if org := os.Getenv("GH_TEST_ORG"); org != "" {
		return org
	}
	return "humansintheloop-test-org"
}

func copyToTempGitRepo(t *testing.T, srcDir string) string {
	t.Helper()
	tmpDir := t.TempDir()

	cmd := exec.Command("cp", "-R", srcDir+"/.", tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to copy %s to temp dir: %v\n%s", srcDir, err, output)
	}

	repoName := fmt.Sprintf("e2e-test-%d", time.Now().UnixMilli())
	org := e2eTestOrg()
	fullName := org + "/" + repoName

	ghCreate := exec.Command("gh", "repo", "create", fullName, "--private", "--confirm")
	if output, err := ghCreate.CombinedOutput(); err != nil {
		t.Fatalf("gh repo create failed: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		exec.Command("gh", "repo", "delete", fullName, "--yes").Run()
	})

	remoteURL := fmt.Sprintf("git@github.com:%s.git", fullName)

	for _, gitCmd := range [][]string{
		{"init"},
		{"add", "."},
		{"commit", "-m", "e2e test"},
		{"remote", "add", "origin", remoteURL},
		{"push", "-u", "origin", "main"},
	} {
		cmd := exec.Command("git", gitCmd...)
		cmd.Dir = tmpDir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", gitCmd, err, output)
		}
	}

	return tmpDir
}

func envFileArgs(t *testing.T, projectRoot string) []string {
	t.Helper()
	envFile := filepath.Join(projectRoot, ".env.local")
	if _, err := os.Stat(envFile); err == nil {
		return []string{"--env-file", envFile}
	}
	return nil
}

func createVMFromTempRepo(t *testing.T, binary, workDir, projectRoot string) {
	t.Helper()
	args := []string{"--type", "vm"}
	args = append(args, envFileArgs(t, projectRoot)...)
	args = append(args, "create")
	cmd := exec.Command(binary, args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("VM create output:\n%s", output)
	if err != nil {
		t.Fatalf("VM create failed: %v", err)
	}
}

func destroyVM(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "vm", "destroy")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("VM destroy output:\n%s", output)
	if err != nil {
		t.Logf("VM destroy failed (ignoring): %v", err)
	}
}
