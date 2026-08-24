package ec2

import (
	"fmt"
	"sort"
)

// lockTimeoutFlag makes a concurrent invocation wait on the state lock rather
// than failing immediately.
const lockTimeoutFlag = "-lock-timeout=120s"

// mutatingFlags are the flags every apply and destroy carries, so that neither
// can prompt, and neither can fail fast on a lock another isolarium holds.
func mutatingFlags(vars map[string]string) []string {
	return append([]string{"-auto-approve", "-input=false", lockTimeoutFlag}, varFlags(vars)...)
}

func varFlags(vars map[string]string) []string {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)

	flags := make([]string, 0, len(names))
	for _, name := range names {
		flags = append(flags, fmt.Sprintf("-var=%s=%s", name, vars[name]))
	}
	return flags
}
