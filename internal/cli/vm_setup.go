package cli

import (
	"fmt"
	"os"

	"github.com/humansintheloop-dev/isolarium/internal/config"
	"github.com/humansintheloop-dev/isolarium/internal/git"
	"github.com/humansintheloop-dev/isolarium/internal/github"
	"github.com/humansintheloop-dev/isolarium/internal/lima"
)

func ensureVMRunning(name string) error {
	state := lima.GetVMState(name)
	switch state {
	case "running":
		return nil
	case "stopped":
		fmt.Println("Starting stopped VM...")
		return lima.StartVM(name)
	default:
		return createAndSetupVM(name)
	}
}

type vmSetup struct {
	name string
	cwd  string
}

func createAndSetupVM(name string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	s := vmSetup{name: name, cwd: cwd}

	repoInfo, err := resolveRepoInfo(cwd)
	if err != nil {
		return err
	}

	fmt.Println("Creating Lima VM...")
	if err := lima.CreateVM(name); err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	if err := s.pushAndCloneRepo(repoInfo); err != nil {
		return err
	}

	fmt.Println("Installing Java and Gradle via SDKMAN...")
	if err := lima.InstallUsingSDKMAN(name); err != nil {
		return fmt.Errorf("failed to install Java/Gradle: %w", err)
	}

	if err := s.installWorkflowTools(); err != nil {
		return err
	}

	if err := s.runIsolationScriptsFromPidYaml(); err != nil {
		return err
	}

	fmt.Println("VM created successfully")
	return nil
}

type repoInfo struct {
	remoteURL string
	branch    string
	owner     string
	repo      string
}

func resolveRepoInfo(cwd string) (*repoInfo, error) {
	remoteURL, err := git.GetRemoteURL(cwd)
	if err != nil {
		return nil, fmt.Errorf("not a git repository (or no remote configured): %w", err)
	}

	branch, err := git.GetCurrentBranch(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	owner, repo, err := github.ParseRepoURL(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository URL: %w", err)
	}

	fmt.Printf("Repository: %s\n", remoteURL)
	fmt.Printf("Branch: %s\n", branch)

	return &repoInfo{remoteURL: remoteURL, branch: branch, owner: owner, repo: repo}, nil
}

func (s vmSetup) pushAndCloneRepo(info *repoInfo) error {
	fmt.Printf("Pushing branch %s to remote...\n", info.branch)
	if err := git.PushBranch(s.cwd, info.branch); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}
	return s.cloneRepoIntoVM(info)
}

func (s vmSetup) cloneRepoIntoVM(info *repoInfo) error {
	token, err := mintGitHubToken()
	if err != nil {
		return err
	}

	fmt.Println("Cloning repository...")
	if err := lima.CloneRepo(s.name, s.cwd, info.remoteURL, info.branch, token); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	if err := s.configureVMGitAuthor(); err != nil {
		return err
	}

	return lima.WriteRepoMetadata(s.name, info.owner, info.repo, info.branch)
}

func (s vmSetup) configureVMGitAuthor() error {
	email, err := git.GetUserEmail(s.cwd)
	if err != nil {
		return fmt.Errorf("failed to get git user.email: %w", err)
	}
	userName, err := git.GetUserName(s.cwd)
	if err != nil {
		return fmt.Errorf("failed to get git user.name: %w", err)
	}
	isolationEmail := git.TransformEmailForIsolation(email)
	isolationName := userName + " - i2code"
	fmt.Printf("Configuring git author: %s <%s>\n", isolationName, isolationEmail)
	return lima.ConfigureGitAuthor(s.name, isolationEmail, isolationName)
}

func (s vmSetup) runIsolationScriptsFromPidYaml() error {
	cfg, err := config.LoadPidConfig(s.cwd)
	if err != nil {
		return fmt.Errorf("loading pid.yaml: %w", err)
	}
	if cfg == nil || len(cfg.VM.Create.CreationScripts) == 0 {
		return nil
	}

	homeDir, err := lima.GetVMHomeDir(s.name)
	if err != nil {
		return fmt.Errorf("getting VM home directory: %w", err)
	}

	fmt.Println("Running VM isolation scripts...")
	executor := func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
		return lima.ExecCommand(vm, workdir, envVars, args)
	}
	return lima.RunVMIsolationScripts(cfg.VM.Create.CreationScripts, s.name, homeDir+"/repo", executor)
}

func (s vmSetup) installWorkflowTools() error {
	fmt.Println("Cloning workflow tools...")
	if err := lima.CloneWorkflowTools(s.name, ""); err != nil {
		return fmt.Errorf("failed to clone workflow tools: %w", err)
	}

	fmt.Println("Installing custom plugins...")
	if err := lima.InstallPlugins(s.name); err != nil {
		return fmt.Errorf("failed to install custom plugins: %w", err)
	}

	fmt.Println("Installing i2code CLI...")
	if err := lima.InstallI2Code(s.name); err != nil {
		return fmt.Errorf("failed to install i2code CLI: %w", err)
	}

	return nil
}
