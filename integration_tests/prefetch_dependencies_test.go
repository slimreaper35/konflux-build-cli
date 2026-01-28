package integration_tests

import (
	"testing"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands/prefetch_dependencies"
)

const HermetoImage = "quay.io/konflux-ci/hermeto:latest"

func RunPrefetchDependencies(params prefetch_dependencies.ParamsConfig, imageRegistry ImageRegistry) error {
	return nil
}

func TestPrefetchDependencies(t *testing.T) {
	g := NewWithT(t)

	container := NewBuildCliRunnerContainer("prefetch-dependencies", HermetoImage)
	g.Expect(container.Start()).To(Succeed())
	defer container.Delete()
}
