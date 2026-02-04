package github

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestPrivateKey(t *testing.T) string {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	return string(pemBytes)
}

func TestTokenMinter_CreateJWT(t *testing.T) {
	privateKeyPEM := generateTestPrivateKey(t)
	minter, err := NewTokenMinter("12345", privateKeyPEM, "")
	if err != nil {
		t.Fatalf("failed to create minter: %v", err)
	}

	jwtToken, err := minter.createJWT()
	if err != nil {
		t.Fatalf("failed to create JWT: %v", err)
	}

	// Parse and validate the JWT
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		return &minter.privateKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("failed to parse JWT: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("failed to get claims")
	}

	// Check issuer is app ID
	if claims["iss"] != "12345" {
		t.Errorf("expected iss '12345', got '%v'", claims["iss"])
	}

	// Check expiry is ~10 minutes in the future
	exp, ok := claims["exp"].(float64)
	if !ok {
		t.Fatal("exp claim not found")
	}
	expTime := time.Unix(int64(exp), 0)
	if time.Until(expTime) < 9*time.Minute || time.Until(expTime) > 11*time.Minute {
		t.Errorf("exp should be ~10 minutes in the future, got %v", expTime)
	}
}

func TestTokenMinter_GetInstallationID(t *testing.T) {
	privateKeyPEM := generateTestPrivateKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path
		if r.URL.Path != "/repos/cer/isolarium/installation" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Verify Authorization header has JWT
		auth := r.Header.Get("Authorization")
		if auth == "" || len(auth) < 10 {
			t.Error("missing or invalid Authorization header")
		}

		// Return installation info
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": 98765,
		})
	}))
	defer server.Close()

	minter, err := NewTokenMinter("12345", privateKeyPEM, server.URL)
	if err != nil {
		t.Fatalf("failed to create minter: %v", err)
	}

	installationID, err := minter.getInstallationID("cer", "isolarium")
	if err != nil {
		t.Fatalf("failed to get installation ID: %v", err)
	}

	if installationID != 98765 {
		t.Errorf("expected installation ID 98765, got %d", installationID)
	}
}

func TestTokenMinter_MintInstallationToken(t *testing.T) {
	privateKeyPEM := generateTestPrivateKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/repos/cer/isolarium/installation":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 98765,
			})
		case "/app/installations/98765/access_tokens":
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":      "ghs_test_token_abc123",
				"expires_at": "2024-01-01T00:00:00Z",
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	minter, err := NewTokenMinter("12345", privateKeyPEM, server.URL)
	if err != nil {
		t.Fatalf("failed to create minter: %v", err)
	}

	token, err := minter.MintInstallationToken("cer", "isolarium")
	if err != nil {
		t.Fatalf("failed to mint token: %v", err)
	}

	if token != "ghs_test_token_abc123" {
		t.Errorf("expected token 'ghs_test_token_abc123', got '%s'", token)
	}
}

func TestTokenMinter_AppNotInstalled(t *testing.T) {
	privateKeyPEM := generateTestPrivateKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/cer/private-repo/installation" {
			http.Error(w, `{"message": "Not Found"}`, http.StatusNotFound)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	minter, err := NewTokenMinter("12345", privateKeyPEM, server.URL)
	if err != nil {
		t.Fatalf("failed to create minter: %v", err)
	}

	_, err = minter.MintInstallationToken("cer", "private-repo")
	if err == nil {
		t.Error("expected error when app not installed")
	}
	if err != ErrAppNotInstalled {
		t.Errorf("expected ErrAppNotInstalled, got: %v", err)
	}
}

func TestNewTokenMinter_InvalidPrivateKey(t *testing.T) {
	_, err := NewTokenMinter("12345", "not a valid key", "")
	if err == nil {
		t.Error("expected error for invalid private key")
	}
}
