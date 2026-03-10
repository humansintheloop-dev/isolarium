package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ScriptEntry struct {
	Path string   `yaml:"path"`
	Env  []string `yaml:"env"`
}

type IsolationTypeConfig struct {
	IsolationScripts []ScriptEntry `yaml:"isolation_scripts"`
	HostScripts      []ScriptEntry `yaml:"host_scripts"`
}

type PidConfig struct {
	Container IsolationTypeConfig `yaml:"container"`
	VM        IsolationTypeConfig `yaml:"vm"`
}

type pidFile struct {
	Isolarium PidConfig `yaml:"isolarium"`
}

func LoadPidConfig(workDir string) (*PidConfig, error) {
	path := filepath.Join(workDir, "pid.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading pid.yaml: %w", err)
	}

	var pf pidFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parsing pid.yaml: %w", err)
	}

	cfg := &pf.Isolarium
	if err := validateConfig(cfg, workDir); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateConfig(cfg *PidConfig, workDir string) error {
	sections := []struct {
		name    string
		scripts []ScriptEntry
	}{
		{"container.isolation_scripts", cfg.Container.IsolationScripts},
		{"container.host_scripts", cfg.Container.HostScripts},
		{"vm.isolation_scripts", cfg.VM.IsolationScripts},
		{"vm.host_scripts", cfg.VM.HostScripts},
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("resolving work directory: %w", err)
	}

	for _, section := range sections {
		for i, script := range section.scripts {
			if script.Path == "" {
				return fmt.Errorf("%s[%d]: path is required", section.name, i)
			}

			resolved := filepath.Join(absWorkDir, script.Path)
			resolved = filepath.Clean(resolved)
			if !strings.HasPrefix(resolved, absWorkDir+string(filepath.Separator)) && resolved != absWorkDir {
				return fmt.Errorf("%s[%d]: path %q escapes project root", section.name, i, script.Path)
			}
		}
	}

	return nil
}
