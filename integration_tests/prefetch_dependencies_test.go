package integration_tests

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	. "github.com/onsi/gomega"
)

const hermetoLatestImage = "quay.io/konflux-ci/hermeto:latest"

type prefetchDependenciesTestParams struct {
	Context string
	Input   string
}

func cloneGitRepo(repoURL, branch, tempDir string) (string, error) {
	name := "test-case"

	executor := cliwrappers.NewCliExecutor()
	_, _, _, err := executor.ExecuteInDir(tempDir, "git", "clone", repoURL, name, "--depth=1", "--branch", branch)
	if err != nil {
		return "", err
	}

	return filepath.Join(tempDir, name), nil
}

func runPrefetchDependencies(params prefetchDependenciesTestParams) error {
	var err error

	container := NewBuildCliRunnerContainer("kbc-prefetch-dependencies", hermetoLatestImage)
	defer container.DeleteIfExists()

	container.AddVolumeWithOptions(params.Context, "/workspace", "z")

	err = container.Start()
	if err != nil {
		return err
	}

	args := []string{
		"prefetch-dependencies",
		"--source",
		"/workspace",
		"--output",
		"/workspace/output",
		"--input",
		params.Input,
	}
	return container.ExecuteBuildCli(args...)
}

func TestPrefetchDependenciesWithEmptyInput(t *testing.T) {
	SetupGomega(t)

	tempDir, err := CreateTempDir("prefetch-test-case-")
	Expect(err).ToNot(HaveOccurred())
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	params := prefetchDependenciesTestParams{
		Context: tempDir,
		Input:   "",
	}

	Expect(runPrefetchDependencies(params)).To(Succeed())
}

func TestPrefetchDependenciesWithInput(t *testing.T) {
	SetupGomega(t)

	tempDir, err := CreateTempDir("prefetch-test-case-")
	Expect(err).ToNot(HaveOccurred())
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	repoURL := "https://github.com/hermetoproject/integration-tests.git"
	branch := "cargo/e2e"

	repoDir, err := cloneGitRepo(repoURL, branch, tempDir)
	Expect(err).ToNot(HaveOccurred())

	params := prefetchDependenciesTestParams{
		Context: repoDir,
		Input:   `{"packages": [{"type": "cargo"}]}`,
	}

	Expect(runPrefetchDependencies(params)).To(Succeed())
}
