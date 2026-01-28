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

var Params = map[string]common.Parameter{
	"input": {
		Name:         "input",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "input data specifying package managers and their configuration",
		Required:     false,
	},
	"source": {
		Name:         "source",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "path to the directory with the project source code",
		Required:     false,
	},
	"output": {
		Name:         "output",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "path to the directory where dependencies and SBOM will be stored",
		Required:     false,
	},
	"config-file-content": {
		Name:         "config-file-content",
		TypeKind:     reflect.String,
		DefaultValue: "",
		Usage:        "content of the configuration file in YAML format",
		Required:     false,
	},
	"sbom-format": {
		Name:         "sbom-format",
		TypeKind:     reflect.String,
		DefaultValue: "spdx",
		Usage:        "format of the SBOM to generate (SPDX or CycloneDX)",
		Required:     false,
	},
	"mode": {
		Name:         "mode",
		TypeKind:     reflect.String,
		DefaultValue: "strict",
		Usage:        "treat input requirements as errors (strict) or warnings (permissive)",
		Required:     false,
	},
}

type ParamsConfig struct {
	Input             string                 `paramName:"input"`
	Source            string                 `paramName:"source"`
	Output            string                 `paramName:"output"`
	ConfigFileContent string                 `paramName:"config-file-content"`
	SBOMFormat        cliwrappers.SBOMFormat `paramName:"sbom-format"`
	Mode              cliwrappers.Mode       `paramName:"mode"`
}

type PrefetchDependencies struct {
	Config     *ParamsConfig
	HermetoCli cliwrappers.HermetoCliInterface
}

func New(cmd *cobra.Command) (*PrefetchDependencies, error) {
	config := ParamsConfig{}
	if err := common.ParseParameters(cmd, Params, &config); err != nil {
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
	// Just print the Hermeto version for debugging purposes.
	if err := pd.HermetoCli.Version(); err != nil {
		return err
	}

	if pd.Config.Input == "" {
		log.Warn("Skipping prefetch-dependencies as no input was provided")
		return nil
	}

	modifiedConfigFileContent := DropGoProxyFrom(pd.Config.ConfigFileContent)

	var configFilePath string
	if pd.Config.ConfigFileContent == "" {
		configFilePath = ""
	} else {
		configFilePath = filepath.Join(pd.Config.Source, "config.yaml")
		if err := os.WriteFile(configFilePath, []byte(modifiedConfigFileContent), ReadOnlyFileMode); err != nil {
			return err
		}
	}

	if err := CopyNetrcFile(); err != nil {
		return err
	}

	if err := CopyGitAuthFiles(); err != nil {
		log.Warn("No git auth files found in the workspace")
		if err := GenerateGitAuthContent(pd.Config.Source); err != nil {
			return err
		}
	}

	if err := UpdateTrustStore(); err != nil {
		return err
	}

	decodedJSONInput := ParseInput(pd.Config.Input)
	if ContainsRPM(decodedJSONInput) {
		modifiedInput, err := InjectRPMInput(decodedJSONInput)
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

	fetchDepsParams := cliwrappers.FetchDepsParams{
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

	generateEnvParams := cliwrappers.GenerateEnvParams{
		OutputDir:    pd.Config.Output,
		ForOutputDir: "/cachi2/output",
		Format:       cliwrappers.Env,
		Output:       filepath.Join(pd.Config.Source, "cachi2", "cachi2.env"),
	}
	if err := pd.HermetoCli.GenerateEnv(&generateEnvParams); err != nil {
		return err
	}

	injectFilesParams := cliwrappers.InjectFilesParams{
		OutputDir:    pd.Config.Output,
		ForOutputDir: "/cachi2/output",
	}
	if err := pd.HermetoCli.InjectFiles(&injectFilesParams); err != nil {
		return err
	}

	if err := RenameRepoFiles(pd.Config.Output); err != nil {
		return err
	}

	YoloUnregisterSubscriptionManager()
	return nil
}
