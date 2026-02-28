package main

import (
	"os"

	"github.com/humansintheloop-dev/isolarium/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
