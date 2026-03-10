package docker

import (
	"fmt"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

type Creator struct {
	Runner      command.Runner
	MetadataDir string
	ImageTag    string
	Worktree    *WorktreeConfig
	BuildArgs   map[string]string
}

func (c *Creator) Create(name, workDir, contextDir string) error {
	if err := c.checkDockerAvailable(); err != nil {
		return fmt.Errorf("docker is not installed or not running; install Docker Desktop (macOS) or Docker Engine (Linux) to use container mode: %w", err)
	}

	if err := c.buildImage(contextDir); err != nil {
		return fmt.Errorf("failed to build Docker image: %w", err)
	}

	if err := c.startContainer(name, workDir); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	if err := c.writeMetadata(name, workDir); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

func (c *Creator) checkDockerAvailable() error {
	args := BuildCheckDockerCommand()
	_, err := c.Runner.Run(args[0], args[1:]...)
	return err
}

func (c *Creator) buildImage(contextDir string) error {
	args := BuildImageCommand(c.ImageTag, contextDir, c.Worktree, c.BuildArgs)
	output, err := c.Runner.Run(args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, string(output))
	}
	return nil
}

func (c *Creator) startContainer(name, workDir string) error {
	args := BuildRunCommand(name, workDir, c.ImageTag, c.Worktree)
	_, err := c.Runner.Run(args[0], args[1:]...)
	return err
}

func (c *Creator) writeMetadata(name, workDir string) error {
	store := NewMetadataStore(c.MetadataDir, name)
	return store.Write("container", workDir)
}
