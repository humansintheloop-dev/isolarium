package ec2

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	privateKeyMode = 0600
	publicKeyMode  = 0644
	keyComment     = "isolarium"
)

// EnsureKeypair returns the OpenSSH-formatted public half of the host-side
// Ed25519 keypair, generating and persisting the pair on first use.
func EnsureKeypair(base string) (string, error) {
	if err := os.MkdirAll(EC2Dir(base), scaffoldingDirMode); err != nil {
		return "", fmt.Errorf("creating %s: %w", EC2Dir(base), err)
	}

	if fileExists(PrivateKeyPath(base)) {
		return publicKeyForExistingPrivateKey(base)
	}
	return generateKeypair(base)
}

func publicKeyForExistingPrivateKey(base string) (string, error) {
	pemBytes, err := os.ReadFile(PrivateKeyPath(base))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", PrivateKeyPath(base), err)
	}
	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		return "", fmt.Errorf("parsing %s: %w", PrivateKeyPath(base), err)
	}

	authorizedKey := authorizedKeyLine(signer.PublicKey())
	if err := writePublicKeyIfAbsent(base, authorizedKey); err != nil {
		return "", err
	}
	return authorizedKey, nil
}

func generateKeypair(base string) (string, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generating Ed25519 keypair: %w", err)
	}

	if err := writePrivateKey(base, privateKey); err != nil {
		return "", err
	}

	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("encoding public key: %w", err)
	}
	authorizedKey := authorizedKeyLine(sshPublicKey)
	if err := writePublicKeyIfAbsent(base, authorizedKey); err != nil {
		return "", err
	}
	return authorizedKey, nil
}

func writePrivateKey(base string, privateKey ed25519.PrivateKey) error {
	block, err := ssh.MarshalPrivateKey(privateKey, keyComment)
	if err != nil {
		return fmt.Errorf("encoding private key: %w", err)
	}
	if err := os.WriteFile(PrivateKeyPath(base), pem.EncodeToMemory(block), privateKeyMode); err != nil {
		return fmt.Errorf("writing %s: %w", PrivateKeyPath(base), err)
	}
	if err := os.Chmod(PrivateKeyPath(base), privateKeyMode); err != nil {
		return fmt.Errorf("setting mode on %s: %w", PrivateKeyPath(base), err)
	}
	return nil
}

func writePublicKeyIfAbsent(base, authorizedKey string) error {
	if fileExists(PublicKeyPath(base)) {
		return nil
	}
	if err := os.WriteFile(PublicKeyPath(base), []byte(authorizedKey+"\n"), publicKeyMode); err != nil {
		return fmt.Errorf("writing %s: %w", PublicKeyPath(base), err)
	}
	if err := os.Chmod(PublicKeyPath(base), publicKeyMode); err != nil {
		return fmt.Errorf("setting mode on %s: %w", PublicKeyPath(base), err)
	}
	return nil
}

func authorizedKeyLine(publicKey ssh.PublicKey) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey)))
}
