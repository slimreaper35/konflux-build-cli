package image

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var SignCmd = &cobra.Command{
	Use:   "sign",
	Short: "Sign an image with cosign.",
	Long:  "Sign an image with cosign using keyless signing. See https://www.sigstore.dev",
	Example: `
  konflux-build image sign \
    --image konflux-build-cli:latest \
    --rekor-url https://rekor.sigstore.dev \
    --fulcio-url https://fulcio.sigstore.dev \
    --oidc-issuer https://oidc.sigstore.dev \
    --oidc-token-path /path/to/oidc/token
	`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Logger.Debug("Starting the `image sign` command")
		signCommand, err := commands.NewSignCommand(cmd)
		if err != nil {
			logger.Logger.Fatal(err)
		}
		if err := signCommand.Run(); err != nil {
			logger.Logger.Fatal(err)
		}
		logger.Logger.Debug("Finished the `image sign` command")
	},
}

func init() {
	common.RegisterParameters(SignCmd, commands.SignParamsConfig)
}
