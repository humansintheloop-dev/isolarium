package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/cer/isolarium/internal/status"
	"github.com/spf13/cobra"
)

type EnvironmentLister interface {
	List(nameFilter, typeFilter string) []status.EnvironmentStatus
}

func newStatusCmdWithLister(rootCmd *cobra.Command, nameFlag *string, typeFlag *environmentType, lister EnvironmentLister) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of all isolarium environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			nameFilter := ""
			if rootCmd.PersistentFlags().Changed("name") {
				nameFilter = *nameFlag
			}
			typeFilter := ""
			if rootCmd.PersistentFlags().Changed("type") {
				typeFilter = string(*typeFlag)
			}

			envs := lister.List(nameFilter, typeFilter)

			if len(envs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No environments found")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tSTATE\tDETAILS")
			for _, env := range envs {
				details := formatDetails(env)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", env.Name, env.Type, env.State, details)
			}
			w.Flush()
			return nil
		},
	}
}

func formatDetails(env status.EnvironmentStatus) string {
	switch env.Type {
	case "vm":
		if env.Repository != "" {
			return fmt.Sprintf("%s (%s)", env.Repository, env.Branch)
		}
		return ""
	case "container":
		return env.WorkDirectory
	default:
		return ""
	}
}

