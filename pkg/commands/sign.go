package commands

import (
	"github.com/spf13/cobra"

	"reflect"

	"fmt"
	"os"

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

func (s Sign) Run() error {
	common.LogParameters(SignParamsConfig, s.Params)

	// Cosign does not implement the containers-auth.json path walk.
	// Tekton often injects path-scoped keys like "quay.io/org" into ~/.docker/config.json.
	//
	// Cosign only looks up the exact repo string and the registry host ("quay.io"), so it can miss those credentials.
	//
	// SelectRegistryAuth picks the matching token.
	// * CreateDockerConfigDir rewrites it as a host-only entry under DOCKER_CONFIG/config.json for cosign.
	// * https://github.com/containers/image/blob/main/docs/containers-auth.json.5.md
	// * https://konflux-ci.dev/docs/troubleshooting/registries/#failed-to-push-or-pull-image
	registryAuth, err := common.SelectRegistryAuthFromDefaultAuthFile(s.Params.Image)
	if err != nil {
		return fmt.Errorf("cannot select registry authentication for image %s: %w", s.Params.Image, err)
	}

	dockerConfigDir, err := common.CreateDockerConfigDir(registryAuth)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dockerConfigDir)

	return s.CosignCli.Sign(&cliwrappers.CosignSignArgs{
		Image:         s.Params.Image,
		RekorURL:      s.Params.RekorURL,
		FulcioURL:     s.Params.FulcioURL,
		OIDCIssuer:    s.Params.OIDCIssuer,
		OIDCTokenPath: s.Params.OIDCTokenPath,
	}, dockerConfigDir)
}
