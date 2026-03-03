package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvFile_LoadsVariables(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	keyFile := filepath.Join(tmpDir, "key.pem")
	if err := os.WriteFile(keyFile, []byte("key content"), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	content := fmt.Sprintf(`GITHUB_APP_ID=12345
GITHUB_APP_PRIVATE_KEY_PATH=%s
# This is a comment
ANOTHER_VAR=value with spaces
`, keyFile)
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	_ = os.Unsetenv("GITHUB_APP_ID")
	_ = os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	_ = os.Unsetenv("ANOTHER_VAR")

	if err := LoadEnvFile(envFile); err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}

	if got := os.Getenv("GITHUB_APP_ID"); got != "12345" {
		t.Errorf("GITHUB_APP_ID: expected '12345', got '%s'", got)
	}
	if got := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); got != keyFile {
		t.Errorf("GITHUB_APP_PRIVATE_KEY_PATH: expected '%s', got '%s'", keyFile, got)
	}
	if got := os.Getenv("ANOTHER_VAR"); got != "value with spaces" {
		t.Errorf("ANOTHER_VAR: expected 'value with spaces', got '%s'", got)
	}

	_ = os.Unsetenv("GITHUB_APP_ID")
	_ = os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	_ = os.Unsetenv("ANOTHER_VAR")
}

func TestLoadEnvFile_DoesNotOverrideExisting(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	content := `GITHUB_APP_ID=from-file`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	t.Setenv("GITHUB_APP_ID", "from-environment")

	if err := LoadEnvFile(envFile); err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}

	if got := os.Getenv("GITHUB_APP_ID"); got != "from-environment" {
		t.Errorf("GITHUB_APP_ID should not be overridden: expected 'from-environment', got '%s'", got)
	}
}

func TestLoadEnvFile_HandlesNonexistentFile(t *testing.T) {
	err := LoadEnvFile("/nonexistent/path/.env.local")
	if err != nil {
		t.Errorf("expected no error for missing env file, got: %v", err)
	}
}

func TestLoadEnvFile_ValidatesPathVariablesExist(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	realFile := filepath.Join(tmpDir, "real-key.pem")
	if err := os.WriteFile(realFile, []byte("key content"), 0600); err != nil {
		t.Fatalf("failed to write real file: %v", err)
	}

	content := fmt.Sprintf(`GITHUB_APP_PRIVATE_KEY_PATH=%s
`, realFile)
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	_ = os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")

	err := LoadEnvFile(envFile)
	if err != nil {
		t.Errorf("expected no error when PATH file exists, got: %v", err)
	}

	if got := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); got != realFile {
		t.Errorf("GITHUB_APP_PRIVATE_KEY_PATH: expected '%s', got '%s'", realFile, got)
	}

	_ = os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
}

func TestLoadEnvFile_ErrorsWhenPathVariableFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	nonExistentFile := filepath.Join(tmpDir, "non-existent-key.pem")

	content := fmt.Sprintf(`GITHUB_APP_PRIVATE_KEY_PATH=%s
`, nonExistentFile)
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	_ = os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")

	err := LoadEnvFile(envFile)
	if err == nil {
		t.Error("expected error when PATH file doesn't exist, got nil")
	}

	if err != nil {
		errMsg := err.Error()
		if !strings.Contains(errMsg, "GITHUB_APP_PRIVATE_KEY_PATH") {
			t.Errorf("error should mention variable name, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, nonExistentFile) {
			t.Errorf("error should mention file path, got: %s", errMsg)
		}
	}

	_ = os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
}

func TestLoadEnvFile_ValidatesMultiplePathVariables(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.local")

	keyFile := filepath.Join(tmpDir, "key.pem")
	certFile := filepath.Join(tmpDir, "cert.pem")
	if err := os.WriteFile(keyFile, []byte("key"), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}
	if err := os.WriteFile(certFile, []byte("cert"), 0600); err != nil {
		t.Fatalf("failed to write cert file: %v", err)
	}

	content := fmt.Sprintf(`GITHUB_APP_PRIVATE_KEY_PATH=%s
SOME_CERT_PATH=%s
REGULAR_VAR=not_a_path
`, keyFile, certFile)
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	_ = os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	_ = os.Unsetenv("SOME_CERT_PATH")
	_ = os.Unsetenv("REGULAR_VAR")

	err := LoadEnvFile(envFile)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if got := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); got != keyFile {
		t.Errorf("GITHUB_APP_PRIVATE_KEY_PATH: expected '%s', got '%s'", keyFile, got)
	}
	if got := os.Getenv("SOME_CERT_PATH"); got != certFile {
		t.Errorf("SOME_CERT_PATH: expected '%s', got '%s'", certFile, got)
	}
	if got := os.Getenv("REGULAR_VAR"); got != "not_a_path" {
		t.Errorf("REGULAR_VAR: expected 'not_a_path', got '%s'", got)
	}

	_ = os.Unsetenv("GITHUB_APP_PRIVATE_KEY_PATH")
	_ = os.Unsetenv("SOME_CERT_PATH")
	_ = os.Unsetenv("REGULAR_VAR")
}
