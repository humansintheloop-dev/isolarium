//go:build integration

package lima

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func removeContainer(name string) {
	vmShell("bash", "-lc", "docker rm -f "+name).Run()
}

func runDetachedContainer(t *testing.T, name, portMapping, image string) {
	t.Helper()
	cmd := vmShell("bash", "-lc", "docker run -d --name "+name+" -p "+portMapping+" "+image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker run failed: %v\noutput: %s", err, output)
	}
}

func verifyContainerRunning(t *testing.T, name string) {
	t.Helper()
	cmd := vmShell("bash", "-lc", "docker ps --filter name="+name+" --format '{{.Status}}'")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker ps failed: %v", err)
	}
	if !strings.Contains(string(output), "Up") {
		t.Errorf("expected container to be running, got: %s", output)
	}
}

func verifyPortAccessible(t *testing.T, port string) {
	t.Helper()
	cmd := vmShell("bash", "-lc", "curl -sf localhost:"+port)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("curl localhost:%s failed: %v", port, err)
	}
	if !strings.Contains(string(output), "<!DOCTYPE html>") {
		t.Errorf("expected HTML response, got: %s", output)
	}
}

func TestDockerRootless_RunContainer_Integration(t *testing.T) {
	ensureVMRunning(t)

	containerName := "test-docker-rootless"
	removeContainer(containerName)
	runDetachedContainer(t, containerName, "18080:80", "nginx:alpine")
	verifyContainerRunning(t, containerName)
	verifyPortAccessible(t, "18080")
	removeContainer(containerName)
}

func TestExecCommand_EchoHello_Integration(t *testing.T) {
	ensureVMRunning(t)
	ensureRepoDirExists(t)

	workdir := vmRepoDir(t)

	cmdArgs := BuildExecCommand("isolarium", workdir, nil, []string{"echo", "hello"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("echo hello failed: %v", err)
	}
	if !strings.Contains(string(output), "hello") {
		t.Errorf("expected output to contain 'hello', got: %s", output)
	}

	cmdArgs = BuildExecCommand("isolarium", workdir, nil, []string{"pwd"})
	cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
	output, err = cmd.Output()
	if err != nil {
		t.Fatalf("pwd failed: %v", err)
	}
	if !strings.Contains(string(output), "/repo") {
		t.Errorf("expected pwd to contain '/repo', got: %s", output)
	}
}

func TestExecInteractiveCommand_Integration(t *testing.T) {
	ensureVMRunning(t)
	ensureRepoDirExists(t)

	workdir := vmRepoDir(t)

	cmdArgs := BuildInteractiveExecCommand("isolarium", workdir, nil, []string{"cat"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = strings.NewReader("hello\n")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("interactive cat failed: %v", err)
	}
	if !strings.Contains(string(output), "hello") {
		t.Errorf("expected output to contain 'hello', got: %s", output)
	}
}

func TestExecCommand_WithEnvVars_Integration(t *testing.T) {
	ensureVMRunning(t)
	ensureRepoDirExists(t)

	workdir := vmRepoDir(t)

	envVars := map[string]string{"TEST_VAR": "test_value"}
	cmdArgs := BuildExecCommand("isolarium", workdir, envVars, []string{"printenv", "TEST_VAR"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("printenv TEST_VAR failed: %v", err)
	}
	if !strings.Contains(string(output), "test_value") {
		t.Errorf("expected output to contain 'test_value', got: %s", output)
	}
}

func TestExecCommand_SIGINT_Integration(t *testing.T) {
	ensureVMRunning(t)
	ensureRepoDirExists(t)

	workdir := vmRepoDir(t)

	cmdArgs := BuildExecCommand("isolarium", workdir, nil, []string{"sleep", "3600"})
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep command: %v", err)
	}

	time.AfterFunc(1*time.Second, func() {
		cmd.Process.Signal(syscall.SIGINT)
	})

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process terminated as expected
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("process did not terminate within 5 seconds after SIGINT")
	}
}

func TestGetVMState_Integration(t *testing.T) {
	ensureVMRunning(t)

	state := GetVMState(vmName)
	if state != "running" {
		t.Errorf("expected VM state 'running', got %q", state)
	}
}

func TestOpenShell_Integration(t *testing.T) {
	ensureVMRunning(t)

	cmdArgs := BuildShellCommand("isolarium", "", nil)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = strings.NewReader("echo test\nexit\n")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("shell command failed: %v", err)
	}
	if !strings.Contains(string(output), "test") {
		t.Errorf("expected output to contain 'test', got: %s", output)
	}
}
