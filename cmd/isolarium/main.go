package main

import (
	"fmt"
	"os"

	"github.com/cer/isolarium/internal/status"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "isolarium",
		Short: "Secure execution environment for coding agents",
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current status of isolarium environment",
		Run: func(cmd *cobra.Command, args []string) {
			s := status.GetStatus()
			fmt.Printf("VM: %s\n", s.VMState)
			if s.GitHubAppConfigured {
				fmt.Println("GitHub App: configured")
			} else {
				fmt.Println("GitHub App: not configured")
			}
		},
	}

	rootCmd.AddCommand(statusCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
