package integration_tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"

	. "github.com/onsi/gomega"
)

const (
	hermetoLatestImage             = "quay.io/konflux-ci/hermeto:latest"
	hermetoIntegrationTestsRepoURL = "https://github.com/hermetoproject/integration-tests.git"
)

type prefetchDependenciesTestParams struct {
	Context string
	Input   string
}

func cloneGitRepo(url, branch, output string) error {
	executor := cliwrappers.NewCliExecutor()
	_, _, _, err := executor.Execute("git", "clone", url, output, "--depth=1", "--branch", branch)
	return err
}

func parseSBOM(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var sbom map[string]any
	if err := json.Unmarshal(content, &sbom); err != nil {
		return nil, err
	}

	return sbom, nil
}

// TODO: This could be shared with the integration tests framework.
func setupContext(t *testing.T) string {
	t.Helper()
	tempDir, err := CreateTempDir("prefetch-test-context-")
	if err != nil {
		t.Fatalf("Failed to create test context: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	return tempDir
}

func runPrefetchDependencies(params prefetchDependenciesTestParams) error {
	container := NewBuildCliRunnerContainer("kbc-prefetch-dependencies", hermetoLatestImage)
	defer container.DeleteIfExists()

	container.AddVolumeWithOptions(params.Context, "/workspace", "z")
	container.SetWorkdir("/workspace")
	if err := container.Start(); err != nil {
		return err
	}

	args := []string{
		"prefetch-dependencies",
		"--input", params.Input,
	}
	return container.ExecuteBuildCli(args...)
}

func TestPrefetchDependencies(t *testing.T) {
	SetupGomega(t)

	t.Run("should skip prefetch dependencies if no input is provided", func(t *testing.T) {
		tempDir := setupContext(t)

		params := prefetchDependenciesTestParams{
			Context: tempDir,
			Input:   "",
		}
		Expect(runPrefetchDependencies(params)).To(Succeed())
	})

	t.Run("should prefetch dependencies with RPM input", func(t *testing.T) {
		tempDir := setupContext(t)

		branch := "rpm/e2e"
		repoPath := filepath.Join(tempDir, "repo")
		Expect(cloneGitRepo(hermetoIntegrationTestsRepoURL, branch, repoPath)).To(Succeed())

		params := prefetchDependenciesTestParams{
			Context: repoPath,
			Input:   `{"packages": [{"type": "rpm"}]}`,
		}
		Expect(runPrefetchDependencies(params)).To(Succeed())

		// Check output directory layout.
		depsDir := filepath.Join(repoPath, "prefetch-output", "deps", "rpm")
		info, err := os.Stat(depsDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.IsDir()).To(BeTrue())

		// Check generated environment file.
		envFile := filepath.Join(repoPath, "prefetch.env")
		info, err = os.Stat(envFile)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.Mode().IsRegular()).To(BeTrue())

		// Check generated SBOM file.
		sbomFile := filepath.Join(repoPath, "prefetch-output", "bom.json")
		info, err = os.Stat(sbomFile)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.Mode().IsRegular()).To(BeTrue())

		// Check SPDX SBOM content.
		sbom, err := parseSBOM(sbomFile)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom).To(HaveKey("packages"))
		Expect(sbom["packages"]).ToNot(BeEmpty())

		// Check repo files.
		var hermetoRepoCount int
		var cachi2RepoCount int
		filepath.WalkDir(filepath.Join(repoPath, "prefetch-output"), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && entry.Name() == "hermeto.repo" {
				hermetoRepoCount++
			}
			if !entry.IsDir() && entry.Name() == "cachi2.repo" {
				cachi2RepoCount++
			}
			return nil
		})
		Expect(hermetoRepoCount).To(BeNumerically("==", 0))
		Expect(cachi2RepoCount).To(BeNumerically(">", 0))
	})
}
