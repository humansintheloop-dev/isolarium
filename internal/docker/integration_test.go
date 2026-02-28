//go:build integration

package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

const testContainerName = "isolarium-integration-test"
const testImageTag = "isolarium-integration-test:latest"

func TestContainerLifecycle_Integration(t *testing.T) {
	runner := command.ExecRunner{}
	cleanupContainer(runner)

	workDir := t.TempDir()
	writeMarkerFile(t, workDir)
	contextDir := buildDockerContext(t)
	defer os.RemoveAll(contextDir)

	metadataDir := t.TempDir()

	createContainer(t, runner, metadataDir, workDir, contextDir)
	verifyContainerRunning(t, runner)
	verifyMountedFilesVisible(t, runner)
	destroyContainer(t, runner, metadataDir)
	verifyContainerGone(t, runner)
}

func TestContainerSecurityFlags_Integration(t *testing.T) {
	runner := command.ExecRunner{}
	cleanupContainer(runner)

	workDir := t.TempDir()
	contextDir := buildDockerContext(t)
	defer os.RemoveAll(contextDir)

	metadataDir := t.TempDir()

	createContainer(t, runner, metadataDir, workDir, contextDir)
	defer cleanupContainer(runner)

	verifyNonRootUser(t, runner)
	verifyCapabilitiesDropped(t, runner)
}

func writeMarkerFile(t *testing.T, workDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workDir, "marker.txt"), []byte("integration-test"), 0644); err != nil {
		t.Fatalf("failed to write marker file: %v", err)
	}
}

func buildDockerContext(t *testing.T) string {
	t.Helper()
	dir, err := WriteDockerTempfile()
	if err != nil {
		t.Fatalf("failed to write Docker context: %v", err)
	}
	return dir
}

func createContainer(t *testing.T, runner command.ExecRunner, metadataDir, workDir, contextDir string) {
	t.Helper()
	creator := &Creator{
		Runner:      runner,
		MetadataDir: metadataDir,
		ImageTag:    testImageTag,
	}
	if err := creator.Create(testContainerName, workDir, contextDir); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
}

func verifyContainerRunning(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	checker := &StateChecker{Runner: runner}
	state := checker.GetState(testContainerName)
	if state != "running" {
		t.Fatalf("expected container state 'running', got %q", state)
	}
}

func verifyMountedFilesVisible(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	args := BuildExecCommand(testContainerName, nil, []string{"cat", "/home/isolarium/repo/marker.txt"})
	output, err := runner.Run(args[0], args[1:]...)
	if err != nil {
		t.Fatalf("exec cat marker.txt failed: %v", err)
	}
	if !strings.Contains(string(output), "integration-test") {
		t.Errorf("expected marker file content 'integration-test', got %q", string(output))
	}
}

func destroyContainer(t *testing.T, runner command.ExecRunner, metadataDir string) {
	t.Helper()
	destroyer := &Destroyer{
		Runner:      runner,
		MetadataDir: metadataDir,
	}
	if err := destroyer.Destroy(testContainerName); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
}

func verifyContainerGone(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	checker := &StateChecker{Runner: runner}
	state := checker.GetState(testContainerName)
	if state != "none" {
		t.Errorf("expected container state 'none' after destroy, got %q", state)
	}
}

func verifyNonRootUser(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	args := BuildExecCommand(testContainerName, nil, []string{"whoami"})
	output, err := runner.Run(args[0], args[1:]...)
	if err != nil {
		t.Fatalf("exec whoami failed: %v", err)
	}
	user := strings.TrimSpace(string(output))
	if user == "root" {
		t.Error("container is running as root, expected non-root user")
	}
}

func verifyCapabilitiesDropped(t *testing.T, runner command.ExecRunner) {
	t.Helper()
	args := BuildExecCommand(testContainerName, nil, []string{"cat", "/proc/1/status"})
	output, err := runner.Run(args[0], args[1:]...)
	if err != nil {
		t.Fatalf("exec cat /proc/1/status failed: %v", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			capValue := strings.TrimSpace(strings.TrimPrefix(line, "CapEff:"))
			if capValue != "0000000000000000" {
				t.Errorf("expected all capabilities dropped (CapEff: 0000000000000000), got CapEff: %s", capValue)
			}
			return
		}
	}
	t.Error("CapEff line not found in /proc/1/status")
}

func TestDockerfileCreatesSymlinkHierarchyForWorktreeBuildArgs_Integration(t *testing.T) {
	runner := command.ExecRunner{}
	containerName := "isolarium-worktree-test"
	imageTag := "isolarium-worktree-test:latest"
	runner.Run("docker", "rm", "-f", containerName)

	worktreeHostPath := "/Users/dev/src/myproject-wt-feature"
	mainRepoHostPath := "/Users/dev/src/myproject"

	contextDir := buildDockerContext(t)
	defer os.RemoveAll(contextDir)

	buildImageWithWorktreeArgs(t, runner, imageTag, contextDir, worktreeHostPath, mainRepoHostPath)
	defer runner.Run("docker", "rmi", "-f", imageTag)

	startMinimalContainer(t, runner, containerName, imageTag)
	defer runner.Run("docker", "rm", "-f", containerName)

	verifySymlinkTarget(t, runner, containerName, worktreeHostPath, "/home/isolarium/repo")
	verifySymlinkTarget(t, runner, containerName, mainRepoHostPath, "/home/isolarium/main-repo")
	verifyParentDirectoriesAreMode555(t, runner, containerName, worktreeHostPath)
	verifyParentDirectoriesAreMode555(t, runner, containerName, mainRepoHostPath)
}

func TestDockerfileCreatesNoSymlinksWithoutBuildArgs_Integration(t *testing.T) {
	runner := command.ExecRunner{}
	containerName := "isolarium-no-worktree-test"
	imageTag := "isolarium-no-worktree-test:latest"
	runner.Run("docker", "rm", "-f", containerName)

	contextDir := buildDockerContext(t)
	defer os.RemoveAll(contextDir)

	buildImageWithoutWorktreeArgs(t, runner, imageTag, contextDir)
	defer runner.Run("docker", "rmi", "-f", imageTag)

	startMinimalContainer(t, runner, containerName, imageTag)
	defer runner.Run("docker", "rm", "-f", containerName)

	verifyNoSymlinkAt(t, runner, containerName, "/Users")
}

func buildImageWithWorktreeArgs(t *testing.T, runner command.ExecRunner, imageTag, contextDir, worktreeHostPath, mainRepoHostPath string) {
	t.Helper()
	_, err := runner.Run("docker", "build", "-t", imageTag,
		"--build-arg", "WORKTREE_HOST_PATH="+worktreeHostPath,
		"--build-arg", "MAIN_REPO_HOST_PATH="+mainRepoHostPath,
		contextDir)
	if err != nil {
		t.Fatalf("failed to build image with worktree args: %v", err)
	}
}

func buildImageWithoutWorktreeArgs(t *testing.T, runner command.ExecRunner, imageTag, contextDir string) {
	t.Helper()
	_, err := runner.Run("docker", "build", "-t", imageTag, contextDir)
	if err != nil {
		t.Fatalf("failed to build image without worktree args: %v", err)
	}
}

func startMinimalContainer(t *testing.T, runner command.ExecRunner, containerName, imageTag string) {
	t.Helper()
	_, err := runner.Run("docker", "run", "-d", "--name", containerName, imageTag)
	if err != nil {
		t.Fatalf("failed to start container: %v", err)
	}
}

func verifySymlinkTarget(t *testing.T, runner command.ExecRunner, containerName, symlinkPath, expectedTarget string) {
	t.Helper()
	args := BuildExecCommand(containerName, nil, []string{"readlink", symlinkPath})
	output, err := runner.Run(args[0], args[1:]...)
	if err != nil {
		t.Fatalf("readlink %s failed: %v", symlinkPath, err)
	}
	actual := strings.TrimSpace(string(output))
	if actual != expectedTarget {
		t.Errorf("expected symlink %s -> %s, got -> %s", symlinkPath, expectedTarget, actual)
	}
}

func verifyParentDirectoriesAreMode555(t *testing.T, runner command.ExecRunner, containerName, path string) {
	t.Helper()
	parent := filepath.Dir(path)
	for parent != "/" {
		args := BuildExecCommand(containerName, nil, []string{"stat", "-c", "%a", parent})
		output, err := runner.Run(args[0], args[1:]...)
		if err != nil {
			t.Fatalf("stat %s failed: %v", parent, err)
		}
		mode := strings.TrimSpace(string(output))
		if mode != "555" {
			t.Errorf("expected directory %s to have mode 555, got %s", parent, mode)
		}
		parent = filepath.Dir(parent)
	}
}

func verifyNoSymlinkAt(t *testing.T, runner command.ExecRunner, containerName, path string) {
	t.Helper()
	args := BuildExecCommand(containerName, nil, []string{"test", "-e", path})
	_, err := runner.Run(args[0], args[1:]...)
	if err == nil {
		t.Errorf("expected %s to not exist in container without worktree args", path)
	}
}

func cleanupContainer(runner command.ExecRunner) {
	runner.Run("docker", "rm", "-f", testContainerName)
}

func TestWorktreeGitOperationsWork_Integration(t *testing.T) {
	runner := command.ExecRunner{}
	containerName := "isolarium-worktree-git-ops-test"
	imageTag := "isolarium-worktree-git-ops-test:latest"
	runner.Run("docker", "rm", "-f", containerName)

	repoDir := createTempGitRepoForIntegration(t)
	worktreeDir := createWorktreeForIntegration(t, repoDir)

	contextDir := buildDockerContext(t)
	defer os.RemoveAll(contextDir)

	buildImageWithWorktreeArgs(t, runner, imageTag, contextDir, worktreeDir, repoDir)
	defer runner.Run("docker", "rmi", "-f", imageTag)

	startContainerWithWorktreeMounts(t, runner, containerName, imageTag, worktreeDir, repoDir)
	defer runner.Run("docker", "rm", "-f", containerName)

	verifyGitStatusSucceeds(t, runner, containerName, worktreeDir)
	verifySymlinkExistsAtHostPath(t, runner, containerName, worktreeDir)
}

func createTempGitRepoForIntegration(t *testing.T) string {
	t.Helper()
	dir := resolveSymlinks(t, t.TempDir())
	runGitOrFail(t, dir, "init")
	runGitOrFail(t, dir, "config", "user.email", "test@test.com")
	runGitOrFail(t, dir, "config", "user.name", "Test User")
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	runGitOrFail(t, dir, "add", "test.txt")
	runGitOrFail(t, dir, "commit", "-m", "initial commit")
	return dir
}

func createWorktreeForIntegration(t *testing.T, repoDir string) string {
	t.Helper()
	worktreeDir := filepath.Join(resolveSymlinks(t, t.TempDir()), "my-worktree")
	runGitOrFail(t, repoDir, "worktree", "add", worktreeDir, "-b", "worktree-branch")
	return worktreeDir
}

func runGitOrFail(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func resolveSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("failed to resolve symlinks for %q: %v", path, err)
	}
	return resolved
}

func startContainerWithWorktreeMounts(t *testing.T, runner command.ExecRunner, containerName, imageTag, worktreeDir, mainRepoDir string) {
	t.Helper()
	_, err := runner.Run("docker", "run", "-d",
		"--name", containerName,
		"-v", fmt.Sprintf("%s:/home/isolarium/repo", worktreeDir),
		"-v", fmt.Sprintf("%s:/home/isolarium/main-repo", mainRepoDir),
		imageTag)
	if err != nil {
		t.Fatalf("failed to start container with worktree mounts: %v", err)
	}
}

func verifyGitStatusSucceeds(t *testing.T, runner command.ExecRunner, containerName, worktreeDir string) {
	t.Helper()
	args := BuildExecCommand(containerName, nil, []string{"git", "-C", worktreeDir, "status"})
	output, err := runner.Run(args[0], args[1:]...)
	if err != nil {
		t.Fatalf("git status failed inside container at worktree path %s: %v\noutput: %s", worktreeDir, err, output)
	}
}

func verifySymlinkExistsAtHostPath(t *testing.T, runner command.ExecRunner, containerName, worktreeDir string) {
	t.Helper()
	args := BuildExecCommand(containerName, nil, []string{"ls", "-la", worktreeDir})
	output, err := runner.Run(args[0], args[1:]...)
	if err != nil {
		t.Fatalf("ls -la %s failed inside container: %v", worktreeDir, err)
	}
	if !strings.Contains(string(output), "->") {
		t.Errorf("expected symlink at %s, ls output: %s", worktreeDir, output)
	}
}
