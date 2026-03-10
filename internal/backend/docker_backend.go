package backend

import (
	"fmt"
	"os"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/config"
	"github.com/humansintheloop-dev/isolarium/internal/docker"
	"github.com/humansintheloop-dev/isolarium/internal/git"
)

// ExecFunc is the function signature for executing commands in a container.
type ExecFunc func(name string, envVars map[string]string, args []string) (int, error)

// ShellFunc is the function signature for opening an interactive shell in a container.
type ShellFunc func(name string, envVars map[string]string) (int, error)

// CopyCredentialsFunc is the function signature for copying credentials into a container.
type CopyCredentialsFunc func(name, credentials string) error

// DockerBackend implements the Backend interface using Docker containers.
type DockerBackend struct {
	Runner              command.Runner
	MetadataDir         string
	ImageTag            string
	ContextDirFunc      func() (string, error)
	ExecFunc            ExecFunc
	ExecInteractiveFunc ExecFunc
	OpenShellFunc       ShellFunc
	CopyCredentialsFunc CopyCredentialsFunc
	DetectWorktreeFunc  func(string) (*git.WorktreeInfo, error)
}

func (b *DockerBackend) Create(name string, opts CreateOptions) error {
	contextDir, err := b.ContextDirFunc()
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(contextDir) }()

	creator := &docker.Creator{
		Runner:      b.Runner,
		MetadataDir: b.MetadataDir,
		ImageTag:    b.ImageTag,
	}

	if b.DetectWorktreeFunc != nil {
		wt, err := b.DetectWorktreeFunc(opts.WorkDirectory)
		if err != nil {
			return fmt.Errorf("failed to detect git worktree: %w", err)
		}
		if wt != nil {
			creator.Worktree = &docker.WorktreeConfig{
				WorktreeHostPath: wt.WorktreeDir,
				MainRepoHostPath: wt.MainRepoDir,
				MainRepoDir:      wt.MainRepoDir,
			}
		}
	}

	if err := b.applyIsolationScripts(opts.WorkDirectory, contextDir, creator); err != nil {
		return err
	}

	return creator.Create(name, opts.WorkDirectory, contextDir)
}

func (b *DockerBackend) applyIsolationScripts(workDir, contextDir string, creator *docker.Creator) error {
	cfg, err := config.LoadPidConfig(workDir)
	if err != nil {
		return fmt.Errorf("loading pid.yaml: %w", err)
	}
	if cfg == nil || len(cfg.Container.IsolationScripts) == 0 {
		return nil
	}

	scripts := cfg.Container.IsolationScripts

	buildArgs, err := docker.ValidateAndCollectBuildArgs(scripts)
	if err != nil {
		return err
	}

	if err := docker.PrepareBuildContext(contextDir, workDir, scripts); err != nil {
		return fmt.Errorf("preparing build context: %w", err)
	}

	creator.BuildArgs = buildArgs
	return nil
}

func (b *DockerBackend) Destroy(name string) error {
	destroyer := &docker.Destroyer{
		Runner:      b.Runner,
		MetadataDir: b.MetadataDir,
	}
	return destroyer.Destroy(name)
}

func (b *DockerBackend) Exec(name string, envVars map[string]string, args []string) (int, error) {
	if err := b.ensureContainerRunning(name); err != nil {
		return 1, err
	}
	return b.ExecFunc(name, envVars, args)
}

func (b *DockerBackend) ExecInteractive(name string, envVars map[string]string, args []string) (int, error) {
	if err := b.ensureContainerRunning(name); err != nil {
		return 1, err
	}
	return b.ExecInteractiveFunc(name, envVars, args)
}

func (b *DockerBackend) ensureContainerRunning(name string) error {
	state := b.GetState(name)
	if state == "stopped" {
		return fmt.Errorf("container '%s' is stopped, run 'isolarium create --type container' to recreate it", name)
	}
	return nil
}

func (b *DockerBackend) OpenShell(name string, envVars map[string]string) (int, error) {
	return b.OpenShellFunc(name, envVars)
}

func (b *DockerBackend) GetState(name string) string {
	checker := &docker.StateChecker{Runner: b.Runner}
	return checker.GetState(name)
}

func (b *DockerBackend) CopyCredentials(name string, credentials string) error {
	return b.CopyCredentialsFunc(name, credentials)
}
