package common

import (
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	dockerIOToken      = "docker.io token"
	indexDockerIOToken = "index.docker.io token"
	quayIOKonfluxToken = "quay.io-konflux token"
	quayIOToken        = "quay.io token"
	regIOToken         = "reg.io token"
	regIOFooBarToken   = "reg.io-foo-bar token"
)

func generateDigest() string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	chars := []rune("0123456789abcdef")
	charsLen := len(chars)
	digest := make([]rune, 64)
	for i := range digest {
		digest[i] = chars[rng.Intn(charsLen)]
	}
	return string(digest)
}

func createAuthFile(auths map[string]interface{}) (string, error) {
	tmpDir := os.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	data, err := json.Marshal(auths)
	if err != nil {
		return "", err
	}

	err = os.WriteFile(configPath, data, 0644)
	if err != nil {
		return "", err
	}

	return configPath, nil
}

func TestSelectAuth(t *testing.T) {
	auths := map[string]interface{}{
		"auths": map[string]interface{}{
			"docker.io":                   map[string]string{"auth": dockerIOToken},
			"https://index.docker.io/v1/": map[string]string{"auth": indexDockerIOToken},
			"quay.io/konflux-ci/foo":      map[string]string{"auth": quayIOKonfluxToken},
			"quay.io":                     map[string]string{"auth": quayIOToken},
			"reg.io":                      map[string]string{"auth": regIOToken},
			"reg.io/foo/bar":              map[string]string{"auth": regIOFooBarToken},
		},
	}

	authFile, err := createAuthFile(auths)
	if err != nil {
		t.Fatalf("Failed to create auth file: %v", err)
	}
	defer os.Remove(authFile)

	testCases := []struct {
		imageRef      string
		expectedToken string
	}{
		{"docker.io/library/debian:latest", dockerIOToken},
		{"quay.io", quayIOToken},
		{"quay.io/foo", quayIOToken},
		{"quay.io/foo:0.1", quayIOToken},
		{"quay.io/foo:0.1@sha256:" + generateDigest(), quayIOToken},
		{"quay.io/konflux-ci", quayIOToken},
		{"quay.io/konflux-ci/foo", quayIOKonfluxToken},
		{"quay.io/konflux-ci/foo:0.3", quayIOKonfluxToken},
		{"quay.io/konflux-ci/foo@sha256:" + generateDigest(), quayIOKonfluxToken},
		{"quay.io/konflux-ci/foo:0.3@sha256:" + generateDigest(), quayIOKonfluxToken},
		{"quay.io/konflux-ci/foo/bar", quayIOKonfluxToken},
		{"reg.io", regIOToken},
		{"reg.io/foo", regIOToken},
		{"reg.io/foo/bar", regIOFooBarToken},
		{"new-reg.io/cool-app", "err"},
		{"arbitrary-input", "err"},
	}

	for _, tc := range testCases {
		t.Run(tc.imageRef, func(t *testing.T) {
			registryAuth, err := SelectRegistryAuth(tc.imageRef, authFile)

			if tc.expectedToken == "err" {
				if err == nil {
					t.Errorf("selectRegistryAuth does not return error")
				}
				if !strings.Contains(err.Error(), "Registry authentication is not configured") {
					t.Errorf("selectRegistryAuth does not return error representing token is not found.")
				}
				return
			}

			if registryAuth.Token != tc.expectedToken {
				t.Errorf("Expected token %q, got %q", tc.expectedToken, registryAuth.Token)
			}
		})
	}
}

func TestFallbackSelectionForDockerIO(t *testing.T) {
	auths := map[string]interface{}{
		"auths": map[string]interface{}{
			"https://index.docker.io/v1/": map[string]string{"auth": indexDockerIOToken},
			"quay.io/konflux-ci/foo":      map[string]string{"auth": quayIOKonfluxToken},
			"quay.io":                     map[string]string{"auth": quayIOToken},
		},
	}

	authFile, err := createAuthFile(auths)
	if err != nil {
		t.Fatalf("Failed to create auth file: %v", err)
	}
	defer os.Remove(authFile)

	registryAuth, err := SelectRegistryAuth("docker.io/library/postgres", authFile)

	if err != nil {
		t.Error("Token is not got from auth file.")
		return
	}

	if registryAuth.Registry != registryDockerIO {
		t.Errorf("Token is not selected for registry %s", registryDockerIO)
		return
	}

	if registryAuth.Token != indexDockerIOToken {
		t.Errorf("Token is not selected from registry %s from auth file.", registryIndexDockerIO)
		return
	}
}

func TestExtractCredentials(t *testing.T) {
	testCases := []struct {
		name          string
		token         string
		expectedUser  string
		expectedPass  string
		expectedError bool
	}{
		{
			name:          "valid basic auth",
			token:         base64.StdEncoding.EncodeToString([]byte("user:pass")),
			expectedUser:  "user",
			expectedPass:  "pass",
			expectedError: false,
		},
		{
			name:          "valid auth with complex password",
			token:         base64.StdEncoding.EncodeToString([]byte("admin:p@ssw0rd123!@#")),
			expectedUser:  "admin",
			expectedPass:  "p@ssw0rd123!@#",
			expectedError: false,
		},
		{
			name:          "valid auth with colon in password",
			token:         base64.StdEncoding.EncodeToString([]byte("user:pass:word")),
			expectedUser:  "user",
			expectedPass:  "pass:word",
			expectedError: false,
		},
		{
			name:          "empty username",
			token:         base64.StdEncoding.EncodeToString([]byte(":password")),
			expectedUser:  "",
			expectedPass:  "password",
			expectedError: false,
		},
		{
			name:          "empty password",
			token:         base64.StdEncoding.EncodeToString([]byte("username:")),
			expectedUser:  "username",
			expectedPass:  "",
			expectedError: false,
		},
		{
			name:          "invalid base64",
			token:         "invalid-base64!@#",
			expectedError: true,
		},
		{
			name:          "missing colon separator",
			token:         base64.StdEncoding.EncodeToString([]byte("usernamepassword")),
			expectedError: true,
		},
		{
			name:          "empty token",
			token:         "",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			username, password, err := ExtractCredentials(tc.token)

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if username != tc.expectedUser {
				t.Errorf("Expected username %q, got %q", tc.expectedUser, username)
			}

			if password != tc.expectedPass {
				t.Errorf("Expected password %q, got %q", tc.expectedPass, password)
			}
		})
	}
}
