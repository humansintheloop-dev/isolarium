package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/cer/isolarium/internal/git"
	"github.com/cer/isolarium/internal/github"
	"github.com/cer/isolarium/internal/lima"
	"github.com/cer/isolarium/internal/status"
	"github.com/spf13/cobra"
)

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return nil // File doesn't exist, skip silently
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Validate that _PATH variables reference existing files
			if strings.HasSuffix(key, "_PATH") && value != "" {
				if _, err := os.Stat(value); os.IsNotExist(err) {
					return fmt.Errorf("%s references non-existent file: %s", key, value)
				}
			}

			// Only set if not already set in environment
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
	return nil
}

func main() {
	// Load .env.local if it exists
	if err := loadEnvFile(".env.local"); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading .env.local: %v\n", err)
		os.Exit(1)
	}
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

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create and start a Lima VM for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get current working directory
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			// Check if we're in a git repository by trying to get the remote URL
			remoteURL, err := git.GetRemoteURL(cwd)
			if err != nil {
				return fmt.Errorf("not a git repository (or no remote configured): %w", err)
			}

			// Get current branch
			branch, err := git.GetCurrentBranch(cwd)
			if err != nil {
				return fmt.Errorf("failed to get current branch: %w", err)
			}

			fmt.Printf("Repository: %s\n", remoteURL)
			fmt.Printf("Branch: %s\n", branch)

			// Create the VM
			fmt.Println("Creating Lima VM...")
			if err := lima.CreateVM(); err != nil {
				return fmt.Errorf("failed to create VM: %w", err)
			}

			// Parse owner/repo from URL
			owner, repo, err := github.ParseRepoURL(remoteURL)
			if err != nil {
				return fmt.Errorf("failed to parse repository URL: %w", err)
			}

			// Try to mint a token if GitHub App is configured
			var token string
			appID := os.Getenv("GITHUB_APP_ID")
			privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
			if appID != "" && privateKeyPath != "" {
				privateKeyBytes, err := os.ReadFile(privateKeyPath)
				if err != nil {
					return fmt.Errorf("failed to read private key from %s: %w", privateKeyPath, err)
				}
				fmt.Println("Minting GitHub App token...")
				minter, err := github.NewTokenMinter(appID, string(privateKeyBytes), "")
				if err != nil {
					return fmt.Errorf("failed to create token minter: %w", err)
				}
				token, err = minter.MintInstallationToken(owner, repo)
				if err != nil {
					return fmt.Errorf("failed to mint token: %w", err)
				}
			}

			// Clone the repository
			fmt.Println("Cloning repository...")
			if err := lima.CloneRepo(remoteURL, branch, token); err != nil {
				return fmt.Errorf("failed to clone repository: %w", err)
			}

			// Write metadata
			if err := lima.WriteRepoMetadata(owner, repo, branch); err != nil {
				return fmt.Errorf("failed to write metadata: %w", err)
			}

			// Clone workflow tools repository (public repo, no token needed)
			fmt.Println("Cloning workflow tools...")
			if err := lima.CloneWorkflowTools(""); err != nil {
				return fmt.Errorf("failed to clone workflow tools: %w", err)
			}

			// Install marketplace plugins
			fmt.Println("Installing marketplace plugins...")
			if err := lima.InstallMarketplacePlugins(); err != nil {
				return fmt.Errorf("failed to install marketplace plugins: %w", err)
			}

			// Install custom plugins
			fmt.Println("Installing custom plugins...")
			if err := lima.ReinstallPlugins(); err != nil {
				return fmt.Errorf("failed to install custom plugins: %w", err)
			}

			fmt.Println("VM created successfully")
			return nil
		},
	}

	destroyCmd := &cobra.Command{
		Use:   "destroy",
		Short: "Delete the Lima VM and all its contents",
		RunE: func(cmd *cobra.Command, args []string) error {
			exists, err := lima.VMExists()
			if err != nil {
				return fmt.Errorf("failed to check VM status: %w", err)
			}
			if !exists {
				fmt.Println("no VM to destroy")
				return nil
			}

			fmt.Println("Destroying Lima VM...")
			if err := lima.DestroyVM(); err != nil {
				return fmt.Errorf("failed to destroy VM: %w", err)
			}
			fmt.Println("VM destroyed successfully")
			return nil
		},
	}

	var copySession bool
	var freshLogin bool
	var interactive bool
	runCmd := &cobra.Command{
		Use:   "run [flags] -- command [args...]",
		Short: "Execute a command inside the VM in the repo directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified; use: isolarium run -- <command> [args...]")
			}

			// Check mutual exclusivity
			if freshLogin && cmd.Flags().Changed("copy-session") {
				return fmt.Errorf("--fresh-login and --copy-session are mutually exclusive")
			}

			// fresh-login disables copy-session
			if freshLogin {
				copySession = false
			}

			// Check if VM exists
			exists, err := lima.VMExists()
			if err != nil {
				return fmt.Errorf("failed to check VM status: %w", err)
			}
			if !exists {
				return fmt.Errorf("no VM exists; run 'isolarium create' first")
			}

			// Copy Claude credentials if requested
			if copySession {
				credentialsPath := os.Getenv("CLAUDE_CREDENTIALS_PATH")
				if credentialsPath == "" {
					return fmt.Errorf("CLAUDE_CREDENTIALS_PATH environment variable not set")
				}
				fmt.Println("Copying Claude credentials to VM...")
				if err := lima.CopyClaudeCredentials(credentialsPath); err != nil {
					return fmt.Errorf("failed to copy credentials: %w", err)
				}
			}

			// Mint fresh GitHub token if configured
			envVars := map[string]string{}
			appID := os.Getenv("GITHUB_APP_ID")
			privateKeyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
			if appID != "" && privateKeyPath != "" {
				// Read repo metadata to get owner/repo
				meta, metaErr := lima.ReadRepoMetadata()
				if metaErr != nil {
					return fmt.Errorf("failed to read repo metadata: %w", metaErr)
				}
				privateKeyBytes, readErr := os.ReadFile(privateKeyPath)
				if readErr != nil {
					return fmt.Errorf("failed to read private key from %s: %w", privateKeyPath, readErr)
				}
				fmt.Println("Minting fresh GitHub App token...")
				minter, mintErr := github.NewTokenMinter(appID, string(privateKeyBytes), "")
				if mintErr != nil {
					return fmt.Errorf("failed to create token minter: %w", mintErr)
				}
				token, tokenErr := minter.MintInstallationToken(meta.Owner, meta.Repo)
				if tokenErr != nil {
					return fmt.Errorf("failed to mint token: %w", tokenErr)
				}
				envVars["GIT_TOKEN"] = token
			}

			// Execute the command inside the VM
			homeDir, homeErr := lima.GetVMHomeDir()
			if homeErr != nil {
				return fmt.Errorf("failed to get VM home directory: %w", homeErr)
			}
			workdir := homeDir + "/repo"

			var exitCode int
			if interactive {
				exitCode, err = lima.ExecInteractiveCommand(lima.GetVMName(), workdir, envVars, args)
			} else {
				exitCode, err = lima.ExecCommand(lima.GetVMName(), workdir, envVars, args)
			}
			if err != nil {
				return fmt.Errorf("failed to execute command: %w", err)
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}

			return nil
		},
	}
	runCmd.Flags().BoolVar(&copySession, "copy-session", true, "Copy Claude credentials from host to VM")
	runCmd.Flags().BoolVar(&freshLogin, "fresh-login", false, "Use device code flow for fresh Claude session (disables --copy-session)")
	runCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Attach TTY for interactive commands")

	sshCmd := &cobra.Command{
		Use:   "ssh",
		Short: "Open an interactive shell inside the VM",
		RunE: func(cmd *cobra.Command, args []string) error {
			exists, err := lima.VMExists()
			if err != nil {
				return fmt.Errorf("failed to check VM status: %w", err)
			}
			if !exists {
				return fmt.Errorf("no VM exists; run 'isolarium create' first")
			}

			exitCode, err := lima.OpenShell(lima.GetVMName())
			if err != nil {
				return fmt.Errorf("failed to open shell: %w", err)
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}
			return nil
		},
	}

	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(sshCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
