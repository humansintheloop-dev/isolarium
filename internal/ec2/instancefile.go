package ec2

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	instanceFilePrefix = "instance-"
	instanceFileSuffix = ".tf"
)

func InstanceFileName(name string) string {
	return instanceFilePrefix + name + instanceFileSuffix
}

func InstanceFilePath(base, name string) string {
	return filepath.Join(TerraformDir(base), InstanceFileName(name))
}

func InstanceFileExists(base, name string) bool {
	return fileExists(InstanceFilePath(base, name))
}

func WriteInstanceFile(base, name, userData string) error {
	path := InstanceFilePath(base, name)
	if fileExists(path) {
		return fmt.Errorf("%s already exists; run isolarium destroy --type ec2 --name %s first",
			InstanceFileName(name), name)
	}

	if err := os.MkdirAll(TerraformDir(base), scaffoldingDirMode); err != nil {
		return fmt.Errorf("creating %s: %w", TerraformDir(base), err)
	}
	if err := os.WriteFile(path, []byte(RenderInstanceFile(name, userData)), scaffoldingFileMode); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func RemoveInstanceFile(base, name string) error {
	if err := os.Remove(InstanceFilePath(base, name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", InstanceFilePath(base, name), err)
	}
	return nil
}
