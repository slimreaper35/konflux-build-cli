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

type Params struct {
	Input             string                 `paramName:"input"`
	Source            string                 `paramName:"source"`
	Output            string                 `paramName:"output"`
	ConfigFileContent string                 `paramName:"config-file-content"`
	SBOMFormat        cliwrappers.SBOMFormat `paramName:"sbom-format"`
	Mode              cliwrappers.Mode       `paramName:"mode"`
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

	// Just print the Hermeto version for debugging purposes.
	if err := pd.HermetoCli.Version(); err != nil {
		return err
	}

	if pd.Config.Input == "" {
		log.Warn("No input provided, skipping prefetch-dependencies")
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
		modifiedInput, err := injectRPMInput(decodedJSONInput)
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

	// Important file for hermetic builds.
	envFilePath := filepath.Join(filepath.Dir(pd.Config.Output), "cachi2.env")

	generateEnvParams := cliwrappers.GenerateEnvParams{
		OutputDir:    pd.Config.Output,
		ForOutputDir: "/cachi2/output",
		Format:       cliwrappers.Env,
		Output:       envFilePath,
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

	return renameRepoFiles(pd.Config.Output)
}
