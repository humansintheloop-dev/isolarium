package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/humansintheloop-dev/isolarium/internal/command"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
	"github.com/spf13/cobra"
)

// newEC2Cmd groups the commands that act on the AWS infrastructure every EC2
// environment shares, rather than on one environment.
func newEC2Cmd() *cobra.Command {
	ec2Cmd := &cobra.Command{
		Use:   "ec2",
		Short: "Manage the shared EC2 infrastructure",
	}
	ec2Cmd.AddCommand(newEC2WipeCmd())
	return ec2Cmd
}

func newEC2WipeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wipe",
		Short: "Tear down the shared EC2 infrastructure, retaining the state bucket",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ec2.Wipe(isolariumBaseDir(), wipeDeps(cmd.OutOrStdout(), cmd.ErrOrStderr()))
		},
	}
}

func wipeDeps(out, errWriter io.Writer) ec2.WipeDeps {
	return ec2.WipeDeps{
		Runner:             command.ExecRunner{},
		ResolveAccountFunc: resolveAWSAccountForWipe,
		EnsureKeypairFunc:  ec2.EnsureKeypair,
		DetectPublicIPFunc: func() (string, error) { return ec2.DetectPublicIP(ec2.DefaultHTTPGet) },
		Out:                out,
		ErrWriter:          errWriter,
	}
}

// resolveAWSAccountForWipe bootstraps the remote-state bucket the same way
// create does, because the bootstrap is idempotent and terraform cannot reach
// the state it is about to destroy without it.
func resolveAWSAccountForWipe() (region, bucket string, err error) {
	region, err = ec2.RequireRegion(os.LookupEnv)
	if err != nil {
		return "", "", err
	}
	bucket, err = ec2.BootstrapStateBucket(context.Background(), region)
	if err != nil {
		return "", "", err
	}
	return region, bucket, nil
}

func isolariumBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".isolarium")
}
