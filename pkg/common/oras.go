package common

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

type OrasClientInterface interface {
	Push(remoteRepo *remote.Repository, tag, localFilePath, artifactType string) (string, error)
}

var _ OrasClientInterface = &OrasClient{}

func NewRepository(imageRepo, username, password string) *remote.Repository {
	o, _ := remote.NewRepository(imageRepo)
	o.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
		Credential: auth.StaticCredential(o.Reference.Registry, auth.Credential{
			Username: username,
			Password: password,
		}),
	}
	return o
}

type OrasClient struct{}

func NewOrasClient() *OrasClient {
	return &OrasClient{}
}

// Push pushes a local file to a remote registry as an OCI artifact.
// Local file path must be absolute and not a directory. The OCI artifact has the specified artifact type and tagged by tag.
// Image digest is returned eventually. If any error occured, it will be returned and an empty string is returned as the image digest.
func (o *OrasClient) Push(remoteRepo *remote.Repository, tag, localFilePath, artifactType string) (string, error) {
	if !filepath.IsAbs(localFilePath) {
		return "", fmt.Errorf("File path is not absolute: %s", localFilePath)
	}
	fi, err := os.Stat(localFilePath)
	if err != nil {
		return "", fmt.Errorf("Error on getting file stat from file %s: %w", localFilePath, err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("Pushing a directory is not supported: %s", localFilePath)
	}
	if artifactType == "" {
		return "", fmt.Errorf("Missing artifact type.")
	}

	fileStorePath := os.TempDir()
	fs, err := file.New(fileStorePath)
	if err != nil {
		return "", fmt.Errorf("Error on creating a file store for oras-push: %w", err)
	}
	defer fs.Close()

	ctx := context.Background()
	fileDescriptor, err := fs.Add(ctx, filepath.Base(localFilePath), "", localFilePath)
	if err != nil {
		return "", fmt.Errorf("Error on adding file %s to file storage: %w", localFilePath, err)
	}
	fileDescriptors := []v1.Descriptor{fileDescriptor}

	opts := oras.PackManifestOptions{Layers: fileDescriptors}
	manifestDescriptor, err := oras.PackManifest(ctx, fs, oras.PackManifestVersion1_1, artifactType, opts)
	if err != nil {
		return "", fmt.Errorf("Error on creating manifest: %w", err)
	}
	l.Logger.Infof("Manifest descriptor: %v", manifestDescriptor)

	if err = fs.Tag(ctx, manifestDescriptor, tag); err != nil {
		return "", fmt.Errorf("Error on tagging manifest: %w", err)
	}

	descriptor, err := oras.Copy(ctx, fs, tag, remoteRepo, tag, oras.DefaultCopyOptions)
	if err != nil {
		return "", fmt.Errorf("Error on copying image to registry: %w", err)
	}

	return string(descriptor.Digest), nil
}
