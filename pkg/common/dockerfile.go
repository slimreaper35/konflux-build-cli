package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DockerfileSearchOpts struct {
	// Source directory path containing application source code.
	SourceDir string
	// Build context directory within the source. It defaults to ".".
	ContextDir string
	// Dockerfile within the source. It not specified, it is searched in order
	// of ./Containerfile and ./Dockerfile. Containerfile takes precedence.
	Dockerfile string
}

// SearchDockerfile searches Dockerfile from given source directory.
//
// Dockerfile must be present under the source and possibly the specified build context directory.
// Caller is responsible for determining the source directory is a relative or absolute path.
// SearchDockerfile does not make assumption on it and search just happens under the passed source directory.
//
// Escape from the source directory is checked. If the source itself is a symbolic link,
// SearchDockerfile does not treat it as an error.
//
// If Dockerfile option is not specified, SearchDockerfile searches ./Containerfile by default,
// then the ./Dockerfile if Containerfile is not found.
//
// Returning empty string to indicate neither is found.
func SearchDockerfile(opts DockerfileSearchOpts) (string, error) {
	if opts.SourceDir == "" {
		return "", fmt.Errorf("Missing source directory")
	}
	contextDir := opts.ContextDir
	if contextDir == "" {
		contextDir = "."
	}

	resolvedSourcePath, err := filepath.EvalSymlinks(opts.SourceDir)
	if err != nil {
		return "", fmt.Errorf("Error on evaluating symlink for source %s: %w", opts.SourceDir, err)
	}
	if !strings.HasSuffix(resolvedSourcePath, "/") {
		resolvedSourcePath += "/"
	}

	var _search = func(dockerfile string) (string, error) {
		contextDir = filepath.Join(resolvedSourcePath, contextDir)
		possibleDockerfiles := []string{
			filepath.Join(contextDir, dockerfile),
			filepath.Join(resolvedSourcePath, dockerfile),
		}
		for _, dockerfilePath := range possibleDockerfiles {
			if realPath, err := filepath.EvalSymlinks(dockerfilePath); err != nil {
				if !os.IsNotExist(err) {
					return "", fmt.Errorf("Error on evaluating symlink for Dockerfile path %s: %w", dockerfilePath, err)
				}
			} else {
				if !strings.HasPrefix(realPath, resolvedSourcePath) {
					return "", fmt.Errorf("Dockerfile is outside of the source directory '%s'.", realPath)
				}
				return realPath, nil
			}
		}
		// No Dockerfile is found.
		return "", nil
	}

	if opts.Dockerfile == "" {
		for _, dockerfile := range []string{"./Containerfile", "./Dockerfile"} {
			dockerfilePath, err := _search(dockerfile)
			if err != nil {
				return "", err
			}
			if dockerfilePath != "" {
				return dockerfilePath, nil
			}
		}
		// Tried all. Nothing is found.
		return "", nil
	}

	return _search(opts.Dockerfile)
}
