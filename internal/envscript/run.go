package envscript

import (
	"fmt"
	"os"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

type EnvExecFunc func(envVars map[string]string, args []string) (int, error)

// scriptSection is one pid.yaml section being carried out: the kind of script it
// declares, so a failure names where the script came from, and the environment
// those scripts should see.
type scriptSection struct {
	kind          string
	name          string
	isolationType string
}

// RunEnvScripts runs the post-creation env scripts inside the environment.
func RunEnvScripts(scripts []config.ScriptEntry, name, isolationType string, executor EnvExecFunc) error {
	return scriptSection{"env script", name, isolationType}.run(scripts, executor)
}

// RunCreationScripts runs the creation scripts inside the environment. It is
// separate from RunEnvScripts only so that a failure names the pid.yaml section
// the script was declared in.
func RunCreationScripts(scripts []config.ScriptEntry, name, isolationType string, executor EnvExecFunc) error {
	return scriptSection{"creation script", name, isolationType}.run(scripts, executor)
}

func (s scriptSection) run(scripts []config.ScriptEntry, executor EnvExecFunc) error {
	if len(scripts) == 0 {
		return nil
	}

	for _, script := range scripts {
		envVars, err := s.collectEnvVars(script)
		if err != nil {
			return err
		}

		_, err = executor(envVars, []string{"bash", script.Path})
		if err != nil {
			return fmt.Errorf("%s %s failed: %w", s.kind, script.Path, err)
		}
	}

	return nil
}

func (s scriptSection) collectEnvVars(script config.ScriptEntry) (map[string]string, error) {
	envVars := map[string]string{
		"ISOLARIUM_NAME": s.name,
		"ISOLARIUM_TYPE": s.isolationType,
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
		return nil, fmt.Errorf("missing required environment variables for %s %s: %s", s.kind, script.Path, strings.Join(missing, ", "))
	}

	return envVars, nil
}
