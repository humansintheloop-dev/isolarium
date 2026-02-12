//go:build integration

package claude

import (
	"testing"
)

func TestReadCredentialsFromKeychain_Integration(t *testing.T) {
	creds, err := ReadCredentialsFromKeychain()
	if err != nil {
		t.Fatalf("failed to read credentials from Keychain: %v", err)
	}
	if creds == "" {
		t.Fatal("credentials are empty")
	}
}
