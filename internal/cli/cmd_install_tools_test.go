package cli

import (
	"strings"
	"testing"

	"github.com/cer/isolarium/internal/backend"
)

func TestInstallToolsCommand_RejectsTypeNono(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"install-tools", "--type", "nono"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --type nono is used with install-tools")
	}

	expectedMessage := "install-tools is not supported with --type nono"
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error containing %q, got %q", expectedMessage, err.Error())
	}
}
