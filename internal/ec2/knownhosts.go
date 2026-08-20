package ec2

import (
	"fmt"
	"strings"

	"github.com/humansintheloop-dev/isolarium/internal/command"
)

const sshKeygenBinary = "ssh-keygen"

// EvictKnownHost removes publicDNS from the dedicated known_hosts file, so that
// AWS handing the same DNS name to a different instance later reads as a new
// host rather than as a tampered one. A known_hosts file that was never written
// has nothing to evict.
func EvictKnownHost(base, publicDNS string, runner command.Runner) error {
	if !fileExists(KnownHostsPath(base)) {
		return nil
	}

	output, err := runner.Run(sshKeygenBinary, "-R", publicDNS, "-f", KnownHostsPath(base))
	if err != nil {
		return fmt.Errorf("evicting %s from %s: %w\n%s",
			publicDNS, KnownHostsPath(base), err, strings.TrimSpace(string(output)))
	}
	return nil
}
