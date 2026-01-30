package image

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var BuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build a container image",
	Long: `Build a container image using buildah.

Optionally, push the built image to a registry using the --push flag.

The command outputs the image URL and optionally the image digest (if pushing).

Secret Handling:
  Use --secret-dirs to provide directories containing secret files that should
  be available during the build. Each file in the root of a secret directory is
  added as a 'buildah build --secret' argument.

  Accepts the following forms of arguments:
    src=DIR_PATH[,name=BASENAME][,optional=true|false]
        Makes the files from DIR_PATH available with id=<BASENAME>/<filename>.
        The BASENAME defaults to the basename of DIR_PATH.
        If optional=true and DIR_PATH doesn't exist, it is skipped.

    DIR_PATH
        Equivalent to src=DIR_PATH

  Access the secrets with 'RUN --mount=type=secret,id=<basename>/<filename>'.
  The --mount option makes them available at /run/secrets/<basename>/<filename>
  for that particular RUN instruction.

Examples:
  # Build using auto-detected Containerfile/Dockerfile in current directory
  konflux-build-cli image build -t quay.io/myorg/myimage:latest

  # Build and push to registry
  konflux-build-cli image build -t quay.io/myorg/myimage:latest --push

  # Build with explicit Containerfile and context
  konflux-build-cli image build -f ./Containerfile -c ./myapp -t quay.io/myorg/myimage:v1.0.0

  # Build with secrets from directories
  konflux-build-cli image build -t quay.io/myorg/myimage:latest \
    --secret-dirs /path/to/secrets1 src=/path/to/secrets2,name=certs

  # Build with additional buildah arguments
  konflux-build-cli image build -t quay.io/myorg/myimage:latest -- --compat-volumes --force-rm
`,
	Run: func(cmd *cobra.Command, args []string) {
		l.Logger.Debug("Starting build")
		build, err := commands.NewBuild(cmd, args)
		if err != nil {
			l.Logger.Fatal(err)
		}
		if err := build.Run(); err != nil {
			l.Logger.Fatal(err)
		}
		l.Logger.Debug("Finished build")
	},
}

func init() {
	common.RegisterParameters(BuildCmd, commands.BuildParamsConfig)
}
