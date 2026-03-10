//go:build e2e_gradlew

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGradlewBuildInNono_EndToEnd(t *testing.T) {
	binary := buildGradlewBinary(t)
	springBootDir := filepath.Join(gradlewProjectRoot(t), "testdata", "spring-boot-app")

	gradlewArgs := []string{"--type", "nono", "run", "--no-gh-token", "--", "./gradlew", "clean", "build"}
	if os.Getenv("GRADLEW_WORKAROUND") == "true" {
		gradlewArgs = append(gradlewArgs, "-PuseJavaAgent")
	}
	cmd := exec.Command(binary, gradlewArgs...)
	cmd.Dir = springBootDir
	output, err := cmd.CombinedOutput()
	t.Logf("gradlew output:\n%s", output)
	if err != nil {
		t.Fatalf("gradlew build in nono failed: %v", err)
	}
	if !strings.Contains(string(output), "BUILD SUCCESSFUL") {
		t.Fatal("expected BUILD SUCCESSFUL in output")
	}
}

func gradlewProjectRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func buildGradlewBinary(t *testing.T) string {
	t.Helper()
	root := gradlewProjectRoot(t)
	binaryPath := filepath.Join(root, "bin", "isolarium")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/isolarium")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, output)
	}
	return binaryPath
}
