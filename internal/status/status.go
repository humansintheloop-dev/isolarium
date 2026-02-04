package status

// Status represents the current state of the isolarium environment
type Status struct {
	VMState             string
	GitHubAppConfigured bool
}

// GetStatus returns the current status of the isolarium environment
func GetStatus() Status {
	return Status{
		VMState:             "none",
		GitHubAppConfigured: false,
	}
}
