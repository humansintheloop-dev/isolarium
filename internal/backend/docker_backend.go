package backend

import (
	"fmt"
	"os"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/config"
	"github.com/humansintheloop-dev/isolarium/internal/docker"
	"github.com/humansintheloop-dev/isolarium/internal/git"
	"github.com/humansintheloop-dev/isolarium/internal/hostscript"
)

type ExecFunc func(req ExecRequest) (int, error)
type ShellFunc func(req ExecRequest) (int, error)
type CopyCredentialsFunc func(containerName, credentials string) error

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

func (b *DockerBackend) Create(opts CreateOptions) error {
	name := opts.Name
	contextDir, err := b.ContextDirFunc()
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(contextDir) }()

	creator, err := b.newCreatorForName(name, opts)
	if err != nil {
		return err
	}

	cfg, err := config.LoadPidConfig(opts.WorkDirectory)
	if err != nil {
		return fmt.Errorf("loading pid.yaml: %w", err)
	}

	if err := b.applyIsolationScripts(cfg, opts.WorkDirectory, contextDir, creator); err != nil {
		return err
	}

	b.injectI2CodeVersion(creator)

	fmt.Printf("Building image %s for container %s...\n", creator.ImageTag, name)
	if err := creator.Create(name, opts.WorkDirectory, contextDir); err != nil {
		return err
	}

	if cfg != nil && len(cfg.Container.Create.HostScripts) > 0 {
		return hostscript.RunHostScripts(cfg.Container.Create.HostScripts, opts.WorkDirectory, name, "container")
	}
	return nil
}

func (b *DockerBackend) imageTagForName(name string) string {
	if b.ImageTag != "" {
		return b.ImageTag
	}
	return docker.ImageTagForContainer(name)
}

func (b *DockerBackend) newCreatorForName(name string, opts CreateOptions) (*docker.Creator, error) {
	creator := &docker.Creator{
		Runner:      b.Runner,
		MetadataDir: b.MetadataDir,
		ImageTag:    b.imageTagForName(name),
	}
	if b.DetectWorktreeFunc == nil {
		return creator, nil
	}
	wt, err := b.DetectWorktreeFunc(opts.WorkDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to detect git worktree: %w", err)
	}
	if wt != nil {
		creator.Worktree = &docker.WorktreeConfig{
			WorktreeHostPath: wt.WorktreeDir,
			MainRepoHostPath: wt.MainRepoDir,
			MainRepoDir:      wt.MainRepoDir,
		}
	}
	return creator, nil
}

func (b *DockerBackend) applyIsolationScripts(cfg *config.PidConfig, workDir, contextDir string, creator *docker.Creator) error {
	if cfg == nil || len(cfg.Container.Create.IsolationScripts) == 0 {
		return nil
	}

	scripts := cfg.Container.Create.IsolationScripts

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

func (b *DockerBackend) RebuildIfChanged(opts CreateOptions) (bool, error) {
	name := opts.Name
	contextDir, err := b.ContextDirFunc()
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(contextDir) }()

	creator, err := b.newCreatorForName(name, opts)
	if err != nil {
		return false, err
	}

	cfg, err := config.LoadPidConfig(opts.WorkDirectory)
	if err != nil {
		return false, fmt.Errorf("loading pid.yaml: %w", err)
	}

	if err := b.applyIsolationScripts(cfg, opts.WorkDirectory, contextDir, creator); err != nil {
		return false, err
	}

	b.injectI2CodeVersion(creator)

	oldImageID := b.containerImageID(name)

	if err := creator.BuildImage(contextDir); err != nil {
		return false, fmt.Errorf("failed to build Docker image: %w", err)
	}

	newImageID := b.imageIDForTag(name)
	if oldImageID == newImageID {
		return false, nil
	}

	fmt.Printf("Image %s changed, recreating container %s...\n", creator.ImageTag, name)
	return true, b.recreateContainer(name, opts, creator)
}

func (b *DockerBackend) recreateContainer(name string, opts CreateOptions, creator *docker.Creator) error {
	if err := b.Destroy(name); err != nil {
		return fmt.Errorf("failed to destroy old container: %w", err)
	}
	if err := creator.StartContainer(name, opts.WorkDirectory); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	return creator.WriteMetadata(name, opts.WorkDirectory)
}

func (b *DockerBackend) injectI2CodeVersion(creator *docker.Creator) {
	args := docker.BuildI2CodeHeadSHACommand()
	output, err := b.Runner.Run(args[0], args[1:]...)
	if err != nil {
		return
	}
	sha := strings.Fields(strings.TrimSpace(string(output)))
	if len(sha) == 0 {
		return
	}
	if creator.BuildArgs == nil {
		creator.BuildArgs = make(map[string]string)
	}
	creator.BuildArgs["I2CODE_VERSION"] = sha[0]
}

func (b *DockerBackend) containerImageID(name string) string {
	args := docker.BuildContainerImageIDCommand(name)
	output, err := b.Runner.Run(args[0], args[1:]...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (b *DockerBackend) imageIDForTag(name string) string {
	args := docker.BuildImageIDCommand(b.imageTagForName(name))
	output, err := b.Runner.Run(args[0], args[1:]...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (b *DockerBackend) WorkDirectoryChanged(name string, requestedDir string) bool {
	store := docker.NewMetadataStore(b.MetadataDir, name)
	meta, err := store.Read()
	if err != nil {
		return false
	}
	return meta.WorkDirectory != requestedDir
}

func (b *DockerBackend) Destroy(name string) error {
	destroyer := &docker.Destroyer{
		Runner:      b.Runner,
		MetadataDir: b.MetadataDir,
	}
	return destroyer.Destroy(name)
}

func (b *DockerBackend) Exec(req ExecRequest) (int, error) {
	if err := b.ensureContainerRunning(req.ContainerName); err != nil {
		return 1, err
	}
	return b.ExecFunc(req)
}

func (b *DockerBackend) ExecInteractive(req ExecRequest) (int, error) {
	if err := b.ensureContainerRunning(req.ContainerName); err != nil {
		return 1, err
	}
	return b.ExecInteractiveFunc(req)
}

func (b *DockerBackend) ensureContainerRunning(containerName string) error {
	state := b.GetState(containerName)
	if state == "stopped" {
		return fmt.Errorf("container '%s' is stopped, run 'isolarium create --type container' to recreate it", containerName)
	}
	return nil
}

func (b *DockerBackend) OpenShell(req ExecRequest) (int, error) {
	return b.OpenShellFunc(req)
}

func (b *DockerBackend) GetState(name string) string {
	checker := &docker.StateChecker{Runner: b.Runner}
	return checker.GetState(name)
}

func (b *DockerBackend) CopyCredentials(name string, credentials string) error {
	return b.CopyCredentialsFunc(name, credentials)
}
