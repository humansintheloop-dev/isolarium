package cli

import (
	"fmt"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

const newSessionFlagUsage = "Start an additional tmux session on the instance instead of joining the running one (ec2 only)"

// rejectNewSessionOutsideEC2 keeps the flag off the environment types that have
// no persistent session to add to, rather than accepting it and doing nothing.
func rejectNewSessionOutsideEC2(envType string, newSession bool) error {
	if newSession && envType != "ec2" {
		return fmt.Errorf("--new-session is only supported with --type ec2")
	}
	return nil
}

// applyNewSession reaches past the Backend interface because which tmux session
// an invocation joins is meaningful to one backend only.
func applyNewSession(b backend.Backend, newSession bool) {
	if !newSession {
		return
	}
	if eb, ok := b.(*backend.EC2Backend); ok {
		eb.UseNewSession()
	}
}
