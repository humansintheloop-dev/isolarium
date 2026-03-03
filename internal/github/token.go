package github

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrAppNotInstalled = errors.New("GitHub App is not installed on this repository")

const defaultGitHubAPIURL = "https://api.github.com"

// TokenMinter creates installation access tokens for GitHub App authentication
type TokenMinter struct {
	appID      string
	privateKey *rsa.PrivateKey
	apiURL     string
	httpClient *http.Client
}

// NewTokenMinter creates a new TokenMinter with the given app ID and private key PEM
func NewTokenMinter(appID, privateKeyPEM, apiURL string) (*TokenMinter, error) {
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	if apiURL == "" {
		apiURL = defaultGitHubAPIURL
	}

	return &TokenMinter{
		appID:      appID,
		privateKey: privateKey,
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// MintInstallationToken creates an access token for the specified repository
func (m *TokenMinter) MintInstallationToken(owner, repo string) (string, error) {
	installationID, err := m.getInstallationID(owner, repo)
	if err != nil {
		return "", err
	}

	return m.createAccessToken(installationID)
}

// createJWT creates a JWT signed with the app's private key
func (m *TokenMinter) createJWT() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(), // Allow for clock drift
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": m.appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(m.privateKey)
}

// getInstallationID finds the installation ID for the given repository
func (m *TokenMinter) getInstallationID(owner, repo string) (int64, error) {
	jwtToken, err := m.createJWT()
	if err != nil {
		return 0, fmt.Errorf("failed to create JWT: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/%s/installation", m.apiURL, owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to get installation: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return 0, ErrAppNotInstalled
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.ID, nil
}

// createAccessToken mints an installation access token
func (m *TokenMinter) createAccessToken(installationID int64) (string, error) {
	jwtToken, err := m.createJWT()
	if err != nil {
		return "", fmt.Errorf("failed to create JWT: %w", err)
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", m.apiURL, installationID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create access token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Token, nil
}
