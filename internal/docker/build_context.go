package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

func PrepareBuildContext(contextDir, projectDir string, scripts []config.ScriptEntry) error {
	for _, script := range scripts {
		src := filepath.Join(projectDir, script.Path)
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("copying isolation script %s: %w", script.Path, err)
		}
		dst := filepath.Join(contextDir, filepath.Base(script.Path))
		if err := os.WriteFile(dst, data, 0755); err != nil {
			return fmt.Errorf("writing script to build context %s: %w", filepath.Base(script.Path), err)
		}
	}

	dockerfilePath := filepath.Join(contextDir, "Dockerfile")
	base, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return fmt.Errorf("reading Dockerfile from build context: %w", err)
	}

	generated := GenerateDockerfile(string(base), scripts)
	if err := os.WriteFile(dockerfilePath, []byte(generated), 0644); err != nil {
		return fmt.Errorf("writing generated Dockerfile: %w", err)
	}

	return nil
}

func ValidateAndCollectBuildArgs(scripts []config.ScriptEntry) (map[string]string, error) {
	result := make(map[string]string)
	var missing []string

	for _, script := range scripts {
		for _, env := range script.Env {
			val, ok := os.LookupEnv(env)
			if !ok {
				missing = append(missing, env)
				continue
			}
			result[env] = val
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables for isolation scripts: %s", strings.Join(missing, ", "))
	}

	return result, nil
}
