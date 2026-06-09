package config_test

import (
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/config"

	. "github.com/onsi/gomega"
)

const (
	testGomodProxy = "test-gomod-proxy.io"
	testNpmProxy   = "test-npm-proxy.io"
	testPipProxy   = "test-pip-proxy.io"
	testPnpmProxy  = "test-pnpm-proxy.io"
	testYarnProxy  = "test-yarn-proxy.io"
)

func Test_NewHermetoProxyConfig(t *testing.T) {
	g := NewWithT(t)

	t.Run("should create hermeto proxy config", func(t *testing.T) {
		rawConfig := config.KonfluxRawConfig{
			HermetoPackageRegistryProxyAllowed: "true",
			HermetoGomodProxy:                  testGomodProxy,
			HermetoNpmProxy:                    testNpmProxy,
			HermetoPipProxy:                    testPipProxy,
			HermetoPnpmProxy:                   testPnpmProxy,
			HermetoYarnProxy:                   testYarnProxy,
		}

		hermetoProxyConfig, err := config.NewHermetoProxyConfig(rawConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(hermetoProxyConfig.PackageRegistryProxyAllowed).To(BeTrue())
		g.Expect(hermetoProxyConfig.GomodProxy).To(Equal(testGomodProxy))
		g.Expect(hermetoProxyConfig.NpmProxy).To(Equal(testNpmProxy))
		g.Expect(hermetoProxyConfig.PipProxy).To(Equal(testPipProxy))
		g.Expect(hermetoProxyConfig.PnpmProxy).To(Equal(testPnpmProxy))
		g.Expect(hermetoProxyConfig.YarnProxy).To(Equal(testYarnProxy))
	})

	t.Run("should create hermeto proxy config if parse error happens", func(t *testing.T) {
		rawConfig := config.KonfluxRawConfig{
			HermetoPackageRegistryProxyAllowed: "abcd",
			HermetoGomodProxy:                  testGomodProxy,
			HermetoNpmProxy:                    testNpmProxy,
			HermetoPipProxy:                    testPipProxy,
			HermetoPnpmProxy:                   testPnpmProxy,
			HermetoYarnProxy:                   testYarnProxy,
		}

		hermetoProxyConfig, err := config.NewHermetoProxyConfig(rawConfig)
		g.Expect(err).To(HaveOccurred())
		g.Expect(hermetoProxyConfig.PackageRegistryProxyAllowed).To(BeFalse())
		g.Expect(hermetoProxyConfig.GomodProxy).To(Equal(testGomodProxy))
		g.Expect(hermetoProxyConfig.NpmProxy).To(Equal(testNpmProxy))
		g.Expect(hermetoProxyConfig.PipProxy).To(Equal(testPipProxy))
		g.Expect(hermetoProxyConfig.PnpmProxy).To(Equal(testPnpmProxy))
		g.Expect(hermetoProxyConfig.YarnProxy).To(Equal(testYarnProxy))
	})

	t.Run("should create hermeto proxy config from empty values", func(t *testing.T) {
		rawConfig := config.KonfluxRawConfig{}

		hermetoProxyConfig, err := config.NewHermetoProxyConfig(rawConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(hermetoProxyConfig.PackageRegistryProxyAllowed).To(BeFalse())
		g.Expect(hermetoProxyConfig.GomodProxy).To(BeEmpty())
		g.Expect(hermetoProxyConfig.NpmProxy).To(BeEmpty())
		g.Expect(hermetoProxyConfig.PipProxy).To(BeEmpty())
		g.Expect(hermetoProxyConfig.PnpmProxy).To(BeEmpty())
		g.Expect(hermetoProxyConfig.YarnProxy).To(BeEmpty())
	})
}

func Test_HermetoProxyConfig_DeepCopy(t *testing.T) {
	g := NewWithT(t)

	t.Run("should deep copy hermeto proxy config", func(t *testing.T) {
		hermetoProxyConfig := &config.HermetoProxyConfig{
			PackageRegistryProxyAllowed: true,
			GomodProxy:                  testGomodProxy,
			NpmProxy:                    testNpmProxy,
			PipProxy:                    testPipProxy,
			PnpmProxy:                   testPnpmProxy,
			YarnProxy:                   testYarnProxy,
		}

		HermetoProxyConfigCopy := hermetoProxyConfig.DeepCopy()

		hermetoProxyConfig.PackageRegistryProxyAllowed = false
		hermetoProxyConfig.GomodProxy = "gomod-proxy"
		hermetoProxyConfig.NpmProxy = "npm-proxy"
		hermetoProxyConfig.PipProxy = "pip-proxy"
		hermetoProxyConfig.PnpmProxy = "pnpm-proxy"
		hermetoProxyConfig.YarnProxy = "yarn-proxy"

		g.Expect(HermetoProxyConfigCopy.PackageRegistryProxyAllowed).To(BeTrue())
		g.Expect(HermetoProxyConfigCopy.GomodProxy).To(Equal(testGomodProxy))
		g.Expect(HermetoProxyConfigCopy.NpmProxy).To(Equal(testNpmProxy))
		g.Expect(HermetoProxyConfigCopy.PipProxy).To(Equal(testPipProxy))
		g.Expect(HermetoProxyConfigCopy.PnpmProxy).To(Equal(testPnpmProxy))
		g.Expect(HermetoProxyConfigCopy.YarnProxy).To(Equal(testYarnProxy))
	})
}

func Test_HermetoProxyConfig_ToString(t *testing.T) {
	g := NewWithT(t)

	hermetoProxyConfig := &config.HermetoProxyConfig{
		PackageRegistryProxyAllowed: true,
		GomodProxy:                  "test-gomod-proxy.io",
		NpmProxy:                    "test-npm-proxy.io",
		PipProxy:                    "test-pip-proxy.io",
		PnpmProxy:                   "test-pnpm-proxy.io",
		YarnProxy:                   "test-yarn-proxy.io",
	}

	str := hermetoProxyConfig.ToString()

	g.Expect(str).ToNot(BeEmpty())
}
