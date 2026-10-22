package cliwrappers_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

func mockCosignCli() (*cliwrappers.CosignCli, *mockExecutor) {
	executor := &mockExecutor{}
	cosignCli := &cliwrappers.CosignCli{Executor: executor}
	cliwrappers.DisableRetryer = true
	return cosignCli, executor
}

func TestCosignCliSign(t *testing.T) {
	baseArgs := cliwrappers.CosignSignArgs{
		Image:      "quay.io/org/image:latest",
		RekorURL:   "https://rekor.sigstore.dev",
		FulcioURL:  "https://fulcio.sigstore.dev",
		OIDCIssuer: "https://oidc.sigstore.dev",
	}
	expectedCmdArgs := []string{
		"sign",
		baseArgs.Image,
		"--yes",
		"--use-signing-config=false",
		"--rekor-url", baseArgs.RekorURL,
		"--fulcio-url", baseArgs.FulcioURL,
		"--oidc-issuer", baseArgs.OIDCIssuer,
	}

	t.Run("without extra env", func(t *testing.T) {
		g := NewWithT(t)
		cosignCli, executor := mockCosignCli()

		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("cosign"))
			g.Expect(cmd.Args).To(Equal(expectedCmdArgs))
			g.Expect(cmd.Env).To(BeEmpty())
			return "", "", 0, nil
		}

		g.Expect(cosignCli.Sign(&baseArgs, "")).To(Succeed())
	})

	t.Run("with Docker registry config", func(t *testing.T) {
		g := NewWithT(t)
		cosignCli, executor := mockCosignCli()
		dockerConfigDir := "/tmp/docker-config"

		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("cosign"))
			g.Expect(cmd.Args).To(Equal(expectedCmdArgs))
			g.Expect(cmd.Env).To(Equal([]string{"DOCKER_CONFIG=" + dockerConfigDir}))
			return "", "", 0, nil
		}

		g.Expect(cosignCli.Sign(&baseArgs, dockerConfigDir)).To(Succeed())
	})

	t.Run("with OIDC token", func(t *testing.T) {
		g := NewWithT(t)
		cosignCli, executor := mockCosignCli()

		tokenPath := filepath.Join(t.TempDir(), "token")
		g.Expect(os.WriteFile(tokenPath, []byte("abcdef"), 0o600)).To(Succeed())

		args := baseArgs
		args.OIDCTokenPath = tokenPath

		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("cosign"))
			g.Expect(cmd.Args).To(Equal(expectedCmdArgs))
			g.Expect(cmd.Env).To(Equal([]string{"SIGSTORE_ID_TOKEN=abcdef"}))
			return "", "", 0, nil
		}

		g.Expect(cosignCli.Sign(&args, "")).To(Succeed())
	})
}
