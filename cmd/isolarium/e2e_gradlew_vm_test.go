//go:build e2e_gradlew

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGradlewBuildInVM_EndToEnd(t *testing.T) {
	binary := buildGradlewBinary(t)
	projectRoot := gradlewProjectRoot(t)
	springBootDir := filepath.Join(projectRoot, "testdata", "spring-boot-app")

	tmpDir := copyToTempGitRepo(t, springBootDir)

	createVMForGradlew(t, binary, tmpDir, projectRoot)
	t.Cleanup(func() { destroyVMForGradlew(t, binary, tmpDir) })

	gradlewCmd := "source ~/.sdkman/bin/sdkman-init.sh && ./gradlew clean build"
	gradlewArgs := []string{"--type", "vm", "run", "--no-gh-token", "--copy-session=false", "--", "bash", "-c", gradlewCmd}
	cmd := exec.Command(binary, gradlewArgs...)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	t.Logf("gradlew output:\n%s", output)
	if err != nil {
		t.Fatalf("gradlew build in VM failed: %v", err)
	}
	if !strings.Contains(string(output), "BUILD SUCCESSFUL") {
		t.Fatal("expected BUILD SUCCESSFUL in output")
	}
}

func testOrg() string {
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
	org := testOrg()
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

func createVMForGradlew(t *testing.T, binary, workDir, projectRoot string) {
	t.Helper()
	args := []string{"--type", "vm"}
	envFile := filepath.Join(projectRoot, ".env.local")
	if _, err := os.Stat(envFile); err == nil {
		args = append(args, "--env-file", envFile)
	}
	args = append(args, "create")
	cmd := exec.Command(binary, args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("VM create output:\n%s", output)
	if err != nil {
		t.Fatalf("VM create failed: %v", err)
	}
}

func destroyVMForGradlew(t *testing.T, binary, workDir string) {
	t.Helper()
	cmd := exec.Command(binary, "--type", "vm", "destroy")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	t.Logf("VM destroy output:\n%s", output)
	if err != nil {
		t.Logf("VM destroy failed (ignoring): %v", err)
	}
}
