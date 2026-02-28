package cli

import (
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
)

func TestCloneRepoCommand_RejectsTypeNono(t *testing.T) {
	spy := &backendSpy{}
	rootCmd := newRootCmdWithResolver(func(envType string) (backend.Backend, error) {
		return spy, nil
	})
	rootCmd.SetArgs([]string{"clone-repo", "--type", "nono"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --type nono is used with clone-repo")
	}

	expectedMessage := "clone-repo is not supported with --type nono"
	if !strings.Contains(err.Error(), expectedMessage) {
		t.Errorf("expected error containing %q, got %q", expectedMessage, err.Error())
	}
}
