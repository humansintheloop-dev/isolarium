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

func GetStatus(name string) Status {
	s := Status{
		VMState:             lima.GetVMState(name),
		GitHubAppConfigured: isGitHubAppConfigured(),
	}

	if s.VMState != "none" {
		meta, err := lima.ReadRepoMetadata(name)
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
