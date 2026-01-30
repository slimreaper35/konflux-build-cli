package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var dockerfileContent = []byte("FROM fedora")

type TestCase struct {
	name               string
	searchOpts         DockerfileSearchOpts
	expectedDockerfile string
	setup              func(*testing.T, *TestCase)
}

func createDir(t *testing.T, dirName ...string) string {
	path := filepath.Join(dirName...)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		t.Fatalf("Failed to create base directory: %v", err)
	}
	return path
}

func writeFile(t *testing.T, path string, content []byte) {
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("Failed to create escape target file: %v", err)
	}
}

func TestSearchDockerfileErrorOnEscapingFromSource(t *testing.T) {
	testCases := []TestCase{
		// Tests Dockerfile is escaped by context directory
		// Directory structure:
		// /tmp/workspace/source/dockerfiles(context)/ links to /tmp/workspace/escaped/
		// Search /tmp/workspace/source/dockerfiles/Dockerfile, where context is dockerfiles
		{
			name: "escaped from context directory",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: "dockerfiles",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				workDir := t.TempDir()
				opts := &tc.searchOpts
				opts.SourceDir = createDir(t, workDir, "source")
				escapedDir := createDir(t, workDir, "escaped")
				writeFile(t, filepath.Join(escapedDir, "Dockerfile"), dockerfileContent)
				// Link contextDir to escapedDir
				linkName := filepath.Join(opts.SourceDir, opts.ContextDir)
				if err := os.Symlink(escapedDir, linkName); err != nil {
					t.Fatalf("Failed to create symlink: %v", err)
				}
			},
		},
		// Tests Dockerfile is escaped from source by Dockerfile itself
		// Directory structure:
		// /tmp/workspace/source/dockerfiles/ links to /tmp/workspace/escaped/
		// Search /tmp/workspace/source/dockerfiles/Dockerfile, where dockerfile is dockerfiles/Dockerfile
		{
			name: "escaped from source by Dockerfile itself",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "dockerfiles/Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				workDir := t.TempDir()
				opts := &tc.searchOpts
				opts.SourceDir = createDir(t, workDir, "source")
				escapedDir := createDir(t, workDir, "escaped")
				writeFile(t, filepath.Join(escapedDir, "Dockerfile"), dockerfileContent)
				// Link directory dockerfiles/ to escapedDir
				linkName := filepath.Join(opts.SourceDir, filepath.Dir(opts.Dockerfile))
				if err := os.Symlink(escapedDir, linkName); err != nil {
					t.Fatalf("Failed to create symlink: %v", err)
				}
			},
		},
		{
			name: "escaped from source by ../Dockerfile",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "../Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				// Directory structure:
				// /tmp/workdir/source
				// /tmp/workdir/source/../Dockerfile, where dockerfile is ../Dockerfile
				opts := &tc.searchOpts
				workDir := t.TempDir()
				opts.SourceDir = createDir(t, workDir, "source")
				writeFile(t, filepath.Join(workDir, "Dockerfile"), dockerfileContent)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t, &tc)

			opts := tc.searchOpts
			result, err := SearchDockerfile(opts)

			if err == nil {
				t.Errorf("Expected error for Dockerfile escaped from base directory, but got result: %s", result)
			}
			errMsg := fmt.Sprintf("Dockerfile %s is not present under source '%s'", opts.Dockerfile, opts.SourceDir)
			if !strings.Contains(err.Error(), errMsg) {
				t.Errorf("Expected error message about escaping from source directory, got: %v", err)
			}
		})
	}
}

func TestSearchDockerfileNotFound(t *testing.T) {
	testCases := []TestCase{
		{
			name: "source does not have Dockerfile",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				tc.searchOpts.SourceDir = t.TempDir()
			},
		},
		{
			name: "Dockerfile is specified with a different name",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "./Containerfile.operator",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, "Dockerfile"), dockerfileContent)
			},
		},
		{
			name: "nonexisting ../Dockerfile",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "../Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t, &tc)
			opts := tc.searchOpts
			result, err := SearchDockerfile(opts)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != "" {
				t.Errorf("Expected Dockerfile %s is not found and empty string is returned, but got: %s", opts.Dockerfile, result)
			}
		})
	}
}

func TestSearchDockerfile(t *testing.T) {
	testCases := []TestCase{
		{
			name: "found from source",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, opts.Dockerfile), dockerfileContent)
			},
			expectedDockerfile: "/Dockerfile",
		},
		{
			name: "found from context",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: "components",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				path := createDir(t, opts.SourceDir, opts.ContextDir)
				writeFile(t, filepath.Join(path, opts.Dockerfile), dockerfileContent)
			},
			expectedDockerfile: "/components/Dockerfile",
		},
		{
			name: "dockerfile includes directory",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "dockerfiles/app",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				path := createDir(t, opts.SourceDir, "dockerfiles")
				writeFile(t, filepath.Join(path, filepath.Base(opts.Dockerfile)), dockerfileContent)
			},
			expectedDockerfile: "/dockerfiles/app",
		},
		{
			name: "Dockerfile within context/ takes precedence",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: "components/app",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, "Dockerfile"), dockerfileContent)
				path := createDir(t, opts.SourceDir, opts.ContextDir)
				writeFile(t, filepath.Join(path, "Dockerfile"), dockerfileContent)
			},
			expectedDockerfile: "/components/app/Dockerfile",
		},
		{
			name: "Searched ./Container by default",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, "Containerfile"), dockerfileContent)
				writeFile(t, filepath.Join(opts.SourceDir, "Dockerfile"), dockerfileContent)
			},
			expectedDockerfile: "/Containerfile",
		},
		{
			name: "Fallback to search ./Dockerfile",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, "Dockerfile"), dockerfileContent)
			},
			expectedDockerfile: "/Dockerfile",
		},
		{
			name: "Ignore symlink source directory",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				// Directory structure:
				// /workdir/outside/
				// /workdir/source/, links to /workdir/outside/
				workDir := t.TempDir()
				outsideDir := createDir(t, workDir, "outside")
				writeFile(t, filepath.Join(outsideDir, "Dockerfile"), dockerfileContent)
				// Link source to outside
				linkName := filepath.Join(workDir, "source")
				if err := os.Symlink(outsideDir, linkName); err != nil {
					t.Fatalf("Failed to create symlink: %v", err)
				}
				tc.searchOpts.SourceDir = linkName
			},
			expectedDockerfile: "/Dockerfile",
		},
		{
			name: "both source and context point to .",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  ".",
				ContextDir: ".",
				Dockerfile: "dockerfiles/app",
			},
			setup: func(t *testing.T, tc *TestCase) {
				curDir, err := os.Getwd()
				if err != nil {
					t.Errorf("Error on getting current working directory: %v", err)
				}
				sourceDir := t.TempDir()
				os.Chdir(sourceDir)
				t.Cleanup(func() {
					os.Chdir(curDir)
				})
				path := createDir(t, sourceDir, "dockerfiles")
				writeFile(t, filepath.Join(path, "app"), dockerfileContent)
			},
			expectedDockerfile: "/dockerfiles/app",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t, &tc)

			result, err := SearchDockerfile(tc.searchOpts)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if tc.name == "Ignore symlink source directory" {
				if !strings.HasSuffix(result, "/outside/Dockerfile") {
					t.Errorf("Expected getting Dockerfile from outside/ directroy, but got: '%s'", result)
				}
			} else {
				absSourceDir, _ := filepath.Abs(tc.searchOpts.SourceDir)
				relativePath := strings.TrimPrefix(result, absSourceDir)
				expected := tc.expectedDockerfile
				if relativePath != expected {
					t.Errorf("Expected getting Dockerfile %s, but got: '%s'", expected, relativePath)
				}
			}
		})
	}
}

func TestSearchDockerfileSourceIsRelativePath(t *testing.T) {
	workDir := t.TempDir()
	sourceDir := createDir(t, workDir, "source")
	writeFile(t, filepath.Join(sourceDir, "Dockerfile"), dockerfileContent)

	curDir, err := os.Getwd()
	if err != nil {
		t.Errorf("Error on creating a temporary directory: %v", err)
	}
	os.Chdir(workDir)
	defer os.Chdir(curDir)

	searchOpts := DockerfileSearchOpts{
		SourceDir:  "source",
		ContextDir: ".",
		Dockerfile: "./Dockerfile",
	}

	result, err := SearchDockerfile(searchOpts)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	expected := filepath.Join(workDir, "source/Dockerfile")
	if result != expected {
		t.Errorf("Expected %s, but got %s", expected, result)
	}
}

func TestSearchDockerfileSourceIsRelativePathButNotChdir(t *testing.T) {
	workDir := t.TempDir()
	sourceDir := createDir(t, workDir, "source")
	writeFile(t, filepath.Join(sourceDir, "Dockerfile"), dockerfileContent)

	searchOpts := DockerfileSearchOpts{
		SourceDir:  "source",
		ContextDir: ".",
		Dockerfile: "./Dockerfile",
	}

	result, err := SearchDockerfile(searchOpts)
	if err == nil {
		t.Errorf("Search is expected to fail, but no error is returned and result is: %s", result)
	}
}
