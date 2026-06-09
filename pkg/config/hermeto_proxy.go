package config

import (
	"fmt"
	"strconv"
)

type HermetoProxyConfig struct {
	PackageRegistryProxyAllowed bool
	GomodProxy                  string
	NpmProxy                    string
	PipProxy                    string
	PnpmProxy                   string
	YarnProxy                   string
}

var _ KonfluxConfigPart[*HermetoProxyConfig] = (*HermetoProxyConfig)(nil)

func NewHermetoProxyConfig(rawConfig KonfluxRawConfig) (*HermetoProxyConfig, error) {
	hermetoProxyConfig := &HermetoProxyConfig{
		GomodProxy: rawConfig.HermetoGomodProxy,
		NpmProxy:   rawConfig.HermetoNpmProxy,
		PipProxy:   rawConfig.HermetoPipProxy,
		PnpmProxy:  rawConfig.HermetoPnpmProxy,
		YarnProxy:  rawConfig.HermetoYarnProxy,
	}

	if rawConfig.HermetoPackageRegistryProxyAllowed == "" {
		hermetoProxyConfig.PackageRegistryProxyAllowed = false
		return hermetoProxyConfig, nil
	}

	isPackageRegistryProxyAllowed, err := strconv.ParseBool(rawConfig.HermetoPackageRegistryProxyAllowed)
	hermetoProxyConfig.PackageRegistryProxyAllowed = isPackageRegistryProxyAllowed
	return hermetoProxyConfig, err
}

func (c *HermetoProxyConfig) DeepCopy() *HermetoProxyConfig {
	return &HermetoProxyConfig{
		PackageRegistryProxyAllowed: c.PackageRegistryProxyAllowed,
		GomodProxy:                  c.GomodProxy,
		NpmProxy:                    c.NpmProxy,
		PipProxy:                    c.PipProxy,
		PnpmProxy:                   c.PnpmProxy,
		YarnProxy:                   c.YarnProxy,
	}
}

func (c *HermetoProxyConfig) ToString() string {
	return fmt.Sprintf("%+v", c)
}
