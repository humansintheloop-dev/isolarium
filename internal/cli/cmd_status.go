package cli

import (
	"fmt"

	"github.com/cer/isolarium/internal/status"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current status of isolarium environment",
		Run: func(cmd *cobra.Command, args []string) {
			s := status.GetStatus()
			fmt.Printf("VM: %s\n", s.VMState)
			if s.Repository != "" {
				fmt.Printf("Repository: %s\n", s.Repository)
				fmt.Printf("Branch: %s\n", s.Branch)
			}
			if s.GitHubAppConfigured {
				fmt.Println("GitHub App: configured")
			} else {
				fmt.Println("GitHub App: not configured")
			}
		},
	}
}
