package cmd

import (
	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/commands/prefetch_dependencies"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var log = logger.Logger.WithField("logger", "PrefetchDependencies")

var PrefetchDependenciesCmd = &cobra.Command{
	Use:   "prefetch-dependencies",
	Short: "Prefetch project dependencies",
	Long:  "Prefetch project dependencies using Hermeto to enable hermetic container builds",
	Run: func(cmd *cobra.Command, args []string) {
		prefetchDependencies, err := prefetch_dependencies.New(cmd)
		if err != nil {
			log.Fatal(err)
		}
		if err := prefetchDependencies.Run(); err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	common.RegisterParameters(PrefetchDependenciesCmd, prefetch_dependencies.Params)
}
