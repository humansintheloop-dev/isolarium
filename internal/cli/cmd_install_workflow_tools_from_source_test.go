package cli

import (
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

func TestInstallWorkflowToolsFromSourceCommand_RejectsTypeNono(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"install-workflow-tools-from-source", "--type", "nono", "/some/path"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --type nono is used with install-workflow-tools-from-source")
	}

	expectedMessage := "install-workflow-tools-from-source is not supported with --type nono"
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error containing %q, got %q", expectedMessage, err.Error())
	}
}
