package status

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type EnvironmentStatus struct {
	Name          string
	Type          string
	State         string
	Repository    string
	Branch        string
	WorkDirectory string
}

type StateProvider func(name, envType string) string

type listOptions struct {
	name     string
	envType  string
}

type ListOption func(*listOptions)

func WithName(name string) ListOption {
	return func(o *listOptions) {
		o.name = name
	}
}

func WithType(envType string) ListOption {
	return func(o *listOptions) {
		o.envType = envType
	}
}

var knownTypes = []string{"vm", "container"}

func ListAllEnvironments(baseDir string, stateProvider StateProvider, opts ...ListOption) []EnvironmentStatus {
	options := &listOptions{}
	for _, opt := range opts {
		opt(options)
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}

	var result []EnvironmentStatus
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if options.name != "" && name != options.name {
			continue
		}

		for _, envType := range knownTypes {
			if options.envType != "" && envType != options.envType {
				continue
			}

			typeDir := filepath.Join(baseDir, name, envType)
			if info, err := os.Stat(typeDir); err != nil || !info.IsDir() {
				continue
			}

			env := EnvironmentStatus{
				Name:  name,
				Type:  envType,
				State: stateProvider(name, envType),
			}

			populateTypeSpecificFields(baseDir, name, envType, &env)
			result = append(result, env)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Type < result[j].Type
	})

	return result
}

func populateTypeSpecificFields(baseDir, name, envType string, env *EnvironmentStatus) {
	metadataPath := filepath.Join(baseDir, name, envType, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return
	}

	switch envType {
	case "vm":
		var meta struct {
			Owner  string `json:"owner"`
			Repo   string `json:"repo"`
			Branch string `json:"branch"`
		}
		if err := json.Unmarshal(data, &meta); err == nil && meta.Owner != "" {
			env.Repository = fmt.Sprintf("%s/%s", meta.Owner, meta.Repo)
			env.Branch = meta.Branch
		}
	case "container":
		var meta struct {
			WorkDirectory string `json:"work_directory"`
		}
		if err := json.Unmarshal(data, &meta); err == nil {
			env.WorkDirectory = meta.WorkDirectory
		}
	}
}
