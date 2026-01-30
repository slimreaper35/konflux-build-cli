package prefetch_dependencies

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	"github.com/konflux-ci/konflux-build-cli/pkg/logger"

	"github.com/spf13/cobra"
)

var log = logger.Logger.WithField("logger", "PrefetchDependencies")

var ParamsConfig = map[string]common.Parameter{
	"input": {
		Name:         "input",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "input data specifying package managers and various configuration",
		Required:     false,
	},
	"source": {
		Name:         "source",
		TypeKind:     reflect.String,
		DefaultValue: ".",
		Usage:        "path to the source code directory",
		Required:     false,
	},
	"output": {
		Name:         "output",
		TypeKind:     reflect.String,
		DefaultValue: "./output",
		Usage:        "directory where prefetched dependencies and the SBOM will be stored",
		Required:     false,
	},
	"config-file-content": {
		Name:         "config-file-content",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "YAML configuration file content",
		Required:     false,
	},
	"sbom-format": {
		Name:         "sbom-format",
		TypeKind:     reflect.String,
		DefaultValue: "spdx",
		Usage:        "SBOM format to generate (spdx or cyclonedx)",
		Required:     false,
	},
	"mode": {
		Name:         "mode",
		TypeKind:     reflect.String,
		DefaultValue: "strict",
		Usage:        "how to handle input requirements: strict (fail) or permissive (warn)",
		Required:     false,
	},
	"for-output-dir": {
		Name:         "for-output-dir",
		TypeKind:     reflect.String,
		DefaultValue: "/tmp",
		Usage:        "directory where the output directory will be mounted in the container for hermetic build",
		Required:     false,
	},
	"env-file": {
		Name:         "env-file",
		TypeKind:     reflect.String,
		DefaultValue: "prefetch.env",
		Usage:        "output file with environment variables for hermetic build",
		Required:     false,
	},
	"rhsm-org": {
		Name:         "rhsm-org",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "path to a file with the Red Hat Subscription Manager organization ID",
		Required:     false,
	},
	"rhsm-activation-key": {
		Name:         "rhsm-activation-key",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "path to a file with the Red Hat Subscription Manager activation key",
		Required:     false,
	},
}

type Params struct {
	Input             string `paramName:"input"`
	Source            string `paramName:"source"`
	Output            string `paramName:"output"`
	ConfigFileContent string `paramName:"config-file-content"`
	SBOMFormat        string `paramName:"sbom-format"`
	Mode              string `paramName:"mode"`
	ForOutputDir      string `paramName:"for-output-dir"`
	EnvFile           string `paramName:"env-file"`
	RHSMOrg           string `paramName:"rhsm-org"`
	RHSMActivationKey string `paramName:"rhsm-activation-key"`
}

type PrefetchDependencies struct {
	Config     *Params
	HermetoCli cliwrappers.HermetoCliInterface
}

func New(cmd *cobra.Command) (*PrefetchDependencies, error) {
	config := Params{}
	if err := common.ParseParameters(cmd, ParamsConfig, &config); err != nil {
		return nil, err
	}

	executor := cliwrappers.NewCliExecutor()
	hermetoCli, err := cliwrappers.NewHermetoCli(executor)
	if err != nil {
		return nil, err
	}

	prefetchDependencies := PrefetchDependencies{Config: &config, HermetoCli: hermetoCli}
	return &prefetchDependencies, nil
}

func (pd *PrefetchDependencies) Run() error {
	// Idempotent operation to unregister the subscription manager on exit.
	defer yoloUnregisterSubscriptionManager()

	if err := pd.HermetoCli.Version(); err != nil {
		return err
	}

	if pd.Config.Input == "" {
		log.Warn("No input provided; skipping prefetch-dependencies")
		return nil
	}

	modifiedConfigFileContent := dropGoProxyFrom(pd.Config.ConfigFileContent)

	var configFilePath string
	if pd.Config.ConfigFileContent == "" {
		configFilePath = ""
	} else {
		configFilePath = filepath.Join(pd.Config.Source, "config.yaml")
		if err := os.WriteFile(configFilePath, []byte(modifiedConfigFileContent), readOnlyFileMode); err != nil {
			return err
		}
	}
	defer os.Remove(configFilePath)

	if err := copyNetrcFile(); err != nil {
		return err
	}

	if err := setupGitBasicAuth(pd.Config.Source); err != nil {
		return err
	}

	if err := updateTrustStore(); err != nil {
		return err
	}

	decodedJSONInput := parseInput(pd.Config.Input)
	if containsRPM(decodedJSONInput) {
		modifiedInput, err := injectRPMInput(decodedJSONInput, pd.Config.RHSMOrg, pd.Config.RHSMActivationKey)
		if err != nil {
			return err
		}
		decodedJSONInput = modifiedInput
	}

	encodedJSONInput, err := json.Marshal(decodedJSONInput)
	if err != nil {
		return err
	}

	log.Debugf("Using modified input for Hermeto:\n%s", string(encodedJSONInput))

	fetchDepsParams := cliwrappers.HermetoFetchDepsParams{
		SourceDir:  pd.Config.Source,
		OutputDir:  pd.Config.Output,
		Input:      string(encodedJSONInput),
		ConfigFile: configFilePath,
		SBOMFormat: pd.Config.SBOMFormat,
		Mode:       pd.Config.Mode,
	}
	if err := pd.HermetoCli.FetchDeps(&fetchDepsParams); err != nil {
		return err
	}

	generateEnvParams := cliwrappers.HermetoGenerateEnvParams{
		OutputDir:    pd.Config.Output,
		ForOutputDir: pd.Config.ForOutputDir,
		Format:       "env",
		Output:       pd.Config.EnvFile,
	}
	if err := pd.HermetoCli.GenerateEnv(&generateEnvParams); err != nil {
		return err
	}

	injectFilesParams := cliwrappers.HermetoInjectFilesParams{
		OutputDir:    pd.Config.Output,
		ForOutputDir: pd.Config.ForOutputDir,
	}
	if err := pd.HermetoCli.InjectFiles(&injectFilesParams); err != nil {
		return err
	}

	return renameRepoFiles(pd.Config.Output)
}
