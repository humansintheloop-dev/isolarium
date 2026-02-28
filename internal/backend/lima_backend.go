package backend

import (
	"github.com/humansintheloop-dev/isolarium/internal/lima"
)

// LimaBackend implements the Backend interface using Lima VMs.
type LimaBackend struct{}

func (b *LimaBackend) Create(name string, opts CreateOptions) error {
	return lima.CreateVM(name)
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
