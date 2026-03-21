package envscript

import (
	"fmt"
	"os"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

type EnvExecFunc func(envVars map[string]string, args []string) (int, error)

func RunEnvScripts(scripts []config.ScriptEntry, name, isolationType string, executor EnvExecFunc) error {
	if len(scripts) == 0 {
		return nil
	}

	for _, script := range scripts {
		envVars, err := collectEnvVars(script, name, isolationType)
		if err != nil {
			return err
		}

		_, err = executor(envVars, []string{"bash", script.Path})
		if err != nil {
			return fmt.Errorf("env script %s failed: %w", script.Path, err)
		}
	}

	return nil
}

func collectEnvVars(script config.ScriptEntry, name, isolationType string) (map[string]string, error) {
	envVars := map[string]string{
		"ISOLARIUM_NAME": name,
		"ISOLARIUM_TYPE": isolationType,
	}

	var missing []string
	for _, envName := range script.Env {
		val, ok := os.LookupEnv(envName)
		if !ok {
			missing = append(missing, envName)
			continue
		}
		envVars[envName] = val
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables for env script %s: %s", script.Path, strings.Join(missing, ", "))
	}

	return envVars, nil
}
