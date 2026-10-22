package commands

import (
	"github.com/spf13/cobra"

	"reflect"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
)

var SignParamsConfig = map[string]common.Parameter{
	"image": {
		Name:         "image",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_SIGN_IMAGE",
		DefaultValue: "",
		Usage:        "Container image to sign",
		Required:     true,
	},
	"rekor-url": {
		Name:         "rekor-url",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_SIGN_REKOR_URL",
		DefaultValue: "",
		Usage:        "Rekor transparency log URL",
		Required:     true,
	},
	"fulcio-url": {
		Name:         "fulcio-url",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_SIGN_FULCIO_URL",
		DefaultValue: "",
		Usage:        "Fulcio certificate authority URL",
		Required:     true,
	},
	"oidc-issuer": {
		Name:         "oidc-issuer",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_SIGN_OIDC_ISSUER",
		DefaultValue: "",
		Usage:        "OIDC issuer",
		Required:     true,
	},
	"oidc-token-path": {
		Name:         "oidc-token-path",
		TypeKind:     reflect.String,
		EnvVarName:   "KBC_SIGN_OIDC_TOKEN_PATH",
		DefaultValue: "",
		Usage:        "OIDC token path",
		Required:     true,
	},
}

type Params struct {
	Image         string `paramName:"image"`
	RekorURL      string `paramName:"rekor-url"`
	FulcioURL     string `paramName:"fulcio-url"`
	OIDCIssuer    string `paramName:"oidc-issuer"`
	OIDCTokenPath string `paramName:"oidc-token-path"`
}

type Sign struct {
	Params    *Params
	CosignCli *cliwrappers.CosignCli
}

func NewSignCommand(cmd *cobra.Command) (*Sign, error) {
	params := Params{}
	if err := common.ParseParameters(cmd, SignParamsConfig, &params); err != nil {
		return nil, err
	}

	executor := cliwrappers.NewCliExecutor()
	cosignCli, err := cliwrappers.NewCosignCli(executor)
	if err != nil {
		return nil, err
	}

	return &Sign{Params: &params, CosignCli: cosignCli}, nil
}

func (s *Sign) Run() error {
	return nil
}
