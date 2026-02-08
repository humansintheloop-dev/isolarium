package status

import (
	"fmt"
	"os"

	"github.com/cer/isolarium/internal/lima"
)

// Status represents the current state of the isolarium environment
type Status struct {
	VMState             string
	GitHubAppConfigured bool
	Repository          string
	Branch              string
}

// GetStatus returns the current status of the isolarium environment
func GetStatus() Status {
	s := Status{
		VMState:             lima.GetVMState(),
		GitHubAppConfigured: isGitHubAppConfigured(),
	}

	// Try to read metadata from VM if VM exists
	if s.VMState != "none" {
		meta, err := lima.ReadRepoMetadata()
		if err == nil && meta != nil {
			s.Repository = fmt.Sprintf("%s/%s", meta.Owner, meta.Repo)
			s.Branch = meta.Branch
		}
	}

	return s
}

func isGitHubAppConfigured() bool {
	appID := os.Getenv("GITHUB_APP_ID")
	privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	return appID != "" && privateKeyPath != ""
}
