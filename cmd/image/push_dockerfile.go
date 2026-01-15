package image

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const COMMAND_NAME = "push-dockerfile"

var PushDockerfileCmd = &cobra.Command{
	Use:   fmt.Sprintf("%s", COMMAND_NAME),
	Short: "Discover Dockerfile from source code and push it to registry as an OCI artifact.",
	Long:  "Discover Dockerfile from source code and push it to registry as an OCI artifact.",
	Run: func(cmd *cobra.Command, args []string) {
		l.Logger.Debugf("Starting %s", COMMAND_NAME)
		pushDockerfile, err := commands.NewPushDockerfile(cmd)
		if err != nil {
			l.Logger.Fatal(err)
		}
		if err := pushDockerfile.Run(); err != nil {
			l.Logger.Fatal(err)
		}
		l.Logger.Debugf("Finished %s", COMMAND_NAME)
	},
}

func init() {
	common.RegisterParameters(PushDockerfileCmd, commands.PushDockerfileParamsConfig)
}
