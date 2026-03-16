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

type PostCreationScripts struct {
	HostScripts []ScriptEntry `yaml:"host_scripts"`
	EnvScripts  []ScriptEntry `yaml:"env_scripts"`
}

type CreateConfig struct {
	CreationScripts     []ScriptEntry      `yaml:"creation_scripts"`
	PostCreationScripts PostCreationScripts `yaml:"post_creation_scripts"`
}

type RunConfig struct {
	Env []string `yaml:"env"`
}

type IsolationTypeConfig struct {
	Create CreateConfig `yaml:"create"`
	Run    RunConfig    `yaml:"run"`
}

type PidConfig struct {
	Container IsolationTypeConfig `yaml:"container"`
	VM        IsolationTypeConfig `yaml:"vm"`
	Nono      IsolationTypeConfig `yaml:"nono"`
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
		{"container.create.creation_scripts", cfg.Container.Create.CreationScripts},
		{"container.create.post_creation_scripts.host_scripts", cfg.Container.Create.PostCreationScripts.HostScripts},
		{"container.create.post_creation_scripts.env_scripts", cfg.Container.Create.PostCreationScripts.EnvScripts},
		{"vm.create.creation_scripts", cfg.VM.Create.CreationScripts},
		{"vm.create.post_creation_scripts.host_scripts", cfg.VM.Create.PostCreationScripts.HostScripts},
		{"vm.create.post_creation_scripts.env_scripts", cfg.VM.Create.PostCreationScripts.EnvScripts},
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
