package main

import (
	"fmt"
	"os"

	"github.com/cer/isolarium/internal/cli"
)

func main() {
	if err := cli.LoadEnvFile(".env.local"); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading .env.local: %v\n", err)
		os.Exit(1)
	}

	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
