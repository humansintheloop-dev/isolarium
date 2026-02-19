package nono

import (
	"fmt"

	"github.com/cer/isolarium/internal/command"
)

type Creator struct {
	Runner      command.Runner
	MetadataDir string
}

func (c *Creator) Create(name, workDir string) error {
	if err := c.checkNonoAvailable(); err != nil {
		return fmt.Errorf("nono is not installed. Install nono to use sandbox mode.")
	}

	return c.writeMetadata(name, workDir)
}

func (c *Creator) checkNonoAvailable() error {
	_, err := c.Runner.Run("nono", "--version")
	return err
}

func (c *Creator) writeMetadata(name, workDir string) error {
	store := NewMetadataStore(c.MetadataDir, name)
	return store.Write("nono", workDir)
}
