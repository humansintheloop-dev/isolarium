package docker

import (
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

type StateChecker struct {
	Runner command.Runner
}

func (s *StateChecker) GetState(name string) string {
	args := BuildInspectCommand(name)
	output, err := s.Runner.Run(args[0], args[1:]...)
	if err != nil {
		return "none"
	}
	return ParseContainerState(string(output))
}

func BuildInspectCommand(name string) []string {
	return []string{"docker", "inspect", "--format", "{{.State.Status}}", name}
}

func ParseContainerState(output string) string {
	status := strings.TrimSpace(output)
	switch status {
	case "running":
		return "running"
	case "exited":
		return "stopped"
	default:
		return "none"
	}
}
