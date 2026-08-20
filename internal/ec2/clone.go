package ec2

import (
	"fmt"
	"time"
)

// CloudInitTimeout caps how long create waits for first-boot provisioning. An
// instance that has not finished the toolchain by then is not going to.
const CloudInitTimeout = 15 * time.Minute

const cloudInitPollInterval = 15 * time.Second

// RemoteRunner runs cmd on the instance reachable at publicDNS and returns the
// remote command's exit code.
type RemoteRunner func(base, publicDNS string, cmd RemoteCommand) (int, error)

// SleepFunc is the pause between attempts of a readiness loop, injected so tests
// can spend the budget instantly.
type SleepFunc func(time.Duration)

// WaitForCloudInit blocks until the instance reports that cloud-init finished.
// The retry loop covers the window before sshd accepts connections: until then
// every attempt fails to reach the instance at all, and `cloud-init status
// --wait` only blocks once it does.
func WaitForCloudInit(base, publicDNS string, run RemoteRunner, sleep SleepFunc) error {
	for waited := time.Duration(0); waited < CloudInitTimeout; waited += cloudInitPollInterval {
		finished, err := cloudInitFinished(base, publicDNS, run)
		if err != nil {
			return err
		}
		if finished {
			return nil
		}
		sleep(cloudInitPollInterval)
	}
	return fmt.Errorf("instance did not finish cloud-init within %s", CloudInitTimeout)
}

// cloudInitFinished distinguishes an instance that is not answering yet, which
// is worth retrying, from a transport that cannot be launched at all, which
// never recovers.
func cloudInitFinished(base, publicDNS string, run RemoteRunner) (bool, error) {
	exitCode, err := run(base, publicDNS, RemoteCommand{Args: []string{"cloud-init", "status", "--wait"}})
	if err != nil {
		return false, err
	}
	return exitCode == 0, nil
}
