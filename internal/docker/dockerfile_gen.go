package docker

import (
	"path/filepath"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/config"
)

func GenerateDockerfile(baseDockerfile string, scripts []config.ScriptEntry) string {
	if len(scripts) == 0 {
		return baseDockerfile
	}

	const insertBefore = `CMD ["sleep", "infinity"]`
	idx := strings.Index(baseDockerfile, insertBefore)
	if idx < 0 {
		return baseDockerfile
	}

	var layers strings.Builder
	for _, script := range scripts {
		filename := filepath.Base(script.Path)
		for _, env := range script.Env {
			layers.WriteString("ARG " + env + "\n")
		}
		layers.WriteString("COPY " + filename + " /tmp/" + filename + "\n")
		layers.WriteString("RUN chmod +x /tmp/" + filename + " && /tmp/" + filename + "\n")
	}

	return baseDockerfile[:idx] + layers.String() + baseDockerfile[idx:]
}
