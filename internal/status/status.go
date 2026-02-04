package status

import "os"

// Status represents the current state of the isolarium environment
type Status struct {
	VMState             string
	GitHubAppConfigured bool
}

// GetStatus returns the current status of the isolarium environment
func GetStatus() Status {
	return Status{
		VMState:             "none",
		GitHubAppConfigured: isGitHubAppConfigured(),
	}
}

func isGitHubAppConfigured() bool {
	appID := os.Getenv("GITHUB_APP_ID")
	privateKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	return appID != "" && privateKey != ""
}
