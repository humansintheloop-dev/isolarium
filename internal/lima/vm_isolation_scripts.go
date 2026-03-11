package lima

import (
	"fmt"
	"os"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

type VMExecFunc func(vm, workdir string, envVars map[string]string, args []string) (int, error)

func RunVMIsolationScripts(scripts []config.ScriptEntry, vmName, repoDir string, executor VMExecFunc) error {
	if len(scripts) == 0 {
		return nil
	}

	for _, script := range scripts {
		envVars, err := collectScriptEnvVars(script, vmName)
		if err != nil {
			return err
		}

		_, err = executor(vmName, repoDir, envVars, []string{"bash", script.Path})
		if err != nil {
			return fmt.Errorf("vm isolation script %s failed: %w", script.Path, err)
		}
	}

	return nil
}

func collectScriptEnvVars(script config.ScriptEntry, vmName string) (map[string]string, error) {
	envVars := map[string]string{
		"ISOLARIUM_NAME": vmName,
		"ISOLARIUM_TYPE": "vm",
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
		return nil, fmt.Errorf("missing required environment variables for vm isolation script %s: %s", script.Path, strings.Join(missing, ", "))
	}

	return envVars, nil
}
