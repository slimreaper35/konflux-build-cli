package common

import (
	"path/filepath"
	"strings"
	"testing"

	"oras.land/oras-go/v2/registry/remote"
)

func TestOrasPush(t *testing.T) {
	orasClient := NewOrasClient()

	t.Run("Absolute file path is requried", func(t *testing.T) {
		remoteRepo, _ := remote.NewRepository("quay.io/org/app")
		_, err := orasClient.Push(remoteRepo, "tag", "./source/Dockerfile", "application/vnd.file")
		if err == nil {
			t.Errorf("File path is not absolute: ./source/Dockerfile")
		}
	})

	t.Run("Pushing directory is not supported", func(t *testing.T) {
		someDir := t.TempDir()
		remoteRepo, _ := remote.NewRepository("quay.io/org/app")
		_, err := orasClient.Push(remoteRepo, "tag", someDir, "application/vnd.file")
		if err == nil {
			t.Errorf("Expected error on pushing directory %s. But no error is returned.", someDir)
		}
	})

	t.Run("Input file must exist", func(t *testing.T) {
		someFile := filepath.Join(t.TempDir(), "Containerfile")
		remoteRepo, _ := remote.NewRepository("quay.io/org/app")
		_, err := orasClient.Push(remoteRepo, "tag", someFile, "application/vnd.containerfile")
		if err == nil {
			t.Error("Expected error on missing artifact type. But no error is returned.")
		}
		if !strings.Contains(err.Error(), "Error on getting file stat") {
			t.Errorf("Expected error on getting file stat, but got: %s", err.Error())
		}
	})

	t.Run("Error on missing artifact type", func(t *testing.T) {
		someFile := filepath.Join(t.TempDir(), "Containerfile")
		writeFile(t, someFile, []byte("FROM ubi"))
		remoteRepo, _ := remote.NewRepository("quay.io/org/app")
		_, err := orasClient.Push(remoteRepo, "tag", someFile, "")
		if err == nil {
			t.Error("Expected error on missing artifact type. But no error is returned.")
		}
		if !strings.Contains(err.Error(), "Missing artifact type") {
			t.Errorf("Artifact type is missing, but the returned error seems incorrect: %s", err.Error())
		}
	})
}
