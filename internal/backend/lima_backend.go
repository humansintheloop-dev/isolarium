package backend

import (
	"fmt"

	"github.com/humansintheloop-dev/isolarium/internal/config"
	"github.com/humansintheloop-dev/isolarium/internal/hostscript"
	"github.com/humansintheloop-dev/isolarium/internal/lima"
)

// LimaBackend implements the Backend interface using Lima VMs.
type LimaBackend struct {
	CreateVMFunc   func(name string) error
	VMExecFunc     lima.VMExecFunc
	VMHomeDirFunc  func(name string) (string, error)
}

func (b *LimaBackend) Create(name string, opts CreateOptions) error {
	createVM := b.CreateVMFunc
	if createVM == nil {
		createVM = lima.CreateVM
	}

	if err := createVM(name); err != nil {
		return err
	}

	cfg, err := config.LoadPidConfig(opts.WorkDirectory)
	if err != nil {
		return fmt.Errorf("loading pid.yaml: %w", err)
	}

	if err := b.runIsolationScripts(cfg, name); err != nil {
		return err
	}

	if cfg != nil && len(cfg.VM.Create.HostScripts) > 0 {
		return hostscript.RunHostScripts(cfg.VM.Create.HostScripts, opts.WorkDirectory, name, "vm")
	}
	return nil
}

func (b *LimaBackend) runIsolationScripts(cfg *config.PidConfig, name string) error {
	if cfg == nil || len(cfg.VM.Create.IsolationScripts) == 0 {
		return nil
	}

	executor := b.VMExecFunc
	if executor == nil {
		executor = func(vm, workdir string, envVars map[string]string, args []string) (int, error) {
			return lima.ExecCommand(vm, workdir, envVars, args)
		}
	}

	getHomeDir := b.VMHomeDirFunc
	if getHomeDir == nil {
		getHomeDir = lima.GetVMHomeDir
	}
	homeDir, err := getHomeDir(name)
	if err != nil {
		return fmt.Errorf("getting VM home directory: %w", err)
	}

	return lima.RunVMIsolationScripts(cfg.VM.Create.IsolationScripts, name, homeDir+"/repo", executor)
}

func (b *LimaBackend) Destroy(name string) error {
	return lima.DestroyVM(name)
}

func (b *LimaBackend) Exec(name string, envVars map[string]string, args []string) (int, error) {
	homeDir, err := lima.GetVMHomeDir(name)
	if err != nil {
		return 1, err
	}
	return lima.ExecCommand(name, homeDir, envVars, args)
}

func (b *LimaBackend) ExecInteractive(name string, envVars map[string]string, args []string) (int, error) {
	homeDir, err := lima.GetVMHomeDir(name)
	if err != nil {
		return 1, err
	}
	return lima.ExecInteractiveCommand(name, homeDir, envVars, args)
}

func (b *LimaBackend) OpenShell(name string, envVars map[string]string) (int, error) {
	homeDir, err := lima.GetVMHomeDir(name)
	if err != nil {
		return 1, err
	}
	workdir := homeDir + "/repo"
	return lima.OpenShell(name, workdir, envVars)
}

func (b *LimaBackend) GetState(name string) string {
	return lima.GetVMState(name)
}

func (b *LimaBackend) CopyCredentials(name string, credentials string) error {
	return lima.CopyClaudeCredentials(name, credentials)
}
