package docker

import (
	"fmt"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

type Destroyer struct {
	Runner      command.Runner
	MetadataDir string
}

func (d *Destroyer) Destroy(name string) error {
	if err := d.removeContainer(name); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	if err := d.cleanupMetadata(name); err != nil {
		return fmt.Errorf("failed to cleanup metadata: %w", err)
	}

	return nil
}

func (d *Destroyer) removeContainer(name string) error {
	args := BuildDestroyCommand(name)
	_, err := d.Runner.Run(args[0], args[1:]...)
	return err
}

func (d *Destroyer) cleanupMetadata(name string) error {
	store := NewMetadataStore(d.MetadataDir, name)
	return store.Cleanup()
}

func BuildDestroyCommand(name string) []string {
	return []string{"docker", "rm", "-f", name}
}
