package ec2

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed terraform
var scaffolding embed.FS

const (
	scaffoldingDirMode  = 0755
	scaffoldingFileMode = 0644
)

// ExtractScaffolding writes the embedded Terraform working directory into
// <base>/ec2/terraform, leaving any file the user has already edited untouched.
func ExtractScaffolding(base string) error {
	dir := TerraformDir(base)
	if err := os.MkdirAll(dir, scaffoldingDirMode); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := os.Chmod(dir, scaffoldingDirMode); err != nil {
		return fmt.Errorf("setting mode on %s: %w", dir, err)
	}

	entries, err := scaffolding.ReadDir("terraform")
	if err != nil {
		return fmt.Errorf("reading embedded scaffolding: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := extractScaffoldingFile(entry, dir); err != nil {
			return err
		}
	}
	return nil
}

func extractScaffoldingFile(entry fs.DirEntry, dir string) error {
	target := filepath.Join(dir, entry.Name())
	if fileExists(target) {
		return nil
	}

	content, err := scaffolding.ReadFile(filepath.Join("terraform", entry.Name()))
	if err != nil {
		return fmt.Errorf("reading embedded %s: %w", entry.Name(), err)
	}
	if err := os.WriteFile(target, content, scaffoldingFileMode); err != nil {
		return fmt.Errorf("writing %s: %w", target, err)
	}
	if err := os.Chmod(target, scaffoldingFileMode); err != nil {
		return fmt.Errorf("setting mode on %s: %w", target, err)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
