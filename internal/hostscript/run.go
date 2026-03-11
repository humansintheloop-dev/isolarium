package hostscript

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

func RunHostScripts(scripts []config.ScriptEntry, workDir, name, isolationType string) error {
	if len(scripts) == 0 {
		return nil
	}

	for _, script := range scripts {
		env, err := buildScriptEnv(script, name, isolationType)
		if err != nil {
			return err
		}

		scriptPath := filepath.Join(workDir, script.Path)
		cmd := exec.Command(scriptPath)
		cmd.Dir = workDir
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("host script %s failed: %w", script.Path, err)
		}
	}

	return nil
}

func buildScriptEnv(script config.ScriptEntry, name, isolationType string) ([]string, error) {
	env := os.Environ()
	env = append(env, "ISOLARIUM_NAME="+name)
	env = append(env, "ISOLARIUM_TYPE="+isolationType)

	var missing []string
	for _, envName := range script.Env {
		val, ok := os.LookupEnv(envName)
		if !ok {
			missing = append(missing, envName)
			continue
		}
		env = append(env, envName+"="+val)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables for host script %s: %s", script.Path, strings.Join(missing, ", "))
	}

	return env, nil
}
