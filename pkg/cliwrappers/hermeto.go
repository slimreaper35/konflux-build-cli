// -> https://hermetoproject.github.io/hermeto <-

package cliwrappers

import (
	"errors"
	"strings"

	"github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var log = logger.Logger.WithField("logger", "HermetoCli")

type HermetoCliInterface interface {
	Version() error
	FetchDeps(params *FetchDepsParams) error
	GenerateEnv(params *GenerateEnvParams) error
	InjectFiles(params *InjectFilesParams) error
}

type HermetoCli struct {
	Executor CliExecutorInterface
}

func NewHermetoCli(executor CliExecutorInterface) (*HermetoCli, error) {
	hermetoCliAvailable, err := CheckCliToolAvailable("hermeto")
	if err != nil {
		return nil, err
	}

	if !hermetoCliAvailable {
		return nil, errors.New("Hermeto CLI is not available")
	}

	return &HermetoCli{Executor: executor}, nil
}

// Print the Hermeto version.
func (hc *HermetoCli) Version() error {
	args := []string{"--version"}
	_, _, _, err := hc.Executor.ExecuteWithOutput("hermeto", args...)
	return err
}

type Mode string

const (
	Strict     Mode = "strict"
	Permissive Mode = "permissive"
)

type SBOMFormat string

const (
	SPDX      SBOMFormat = "spdx"
	CycloneDX SBOMFormat = "cyclonedx"
)

type FetchDepsParams struct {
	Input      string
	SourceDir  string
	OutputDir  string
	ConfigFile string
	SBOMFormat SBOMFormat
	Mode       Mode
}

// Run the Hermeto fetch-deps command.
func (hc *HermetoCli) FetchDeps(params *FetchDepsParams) error {
	logLevel := logger.Logger.GetLevel().String()

	args := []string{
		"--log-level",
		logLevel,
		"--mode",
		string(params.Mode),
	}

	// Make the config file optional.
	if params.ConfigFile != "" {
		args = append(args, "--config-file", params.ConfigFile)
	}

	args = append(
		args,
		"fetch-deps",
		params.Input,
		"--sbom-output-type",
		string(params.SBOMFormat),
		"--source",
		params.SourceDir,
		"--output",
		params.OutputDir,
	)

	log.Debugf("Executing hermeto %s", strings.Join(args, " "))
	_, _, _, err := hc.Executor.ExecuteWithOutput("hermeto", args...)
	return err
}

type EnvFileFormat string

const (
	Env  EnvFileFormat = "env"
	Json EnvFileFormat = "json"
)

type GenerateEnvParams struct {
	OutputDir    string
	ForOutputDir string
	Format       EnvFileFormat
	Output       string
}

// Run the Hermeto generate-env command.
func (hc *HermetoCli) GenerateEnv(params *GenerateEnvParams) error {
	logLevel := logger.Logger.GetLevel().String()

	args := []string{
		"--log-level",
		logLevel,
		"generate-env",
		params.OutputDir,
		"--for-output-dir",
		params.ForOutputDir,
		"--format",
		string(params.Format),
		"--output",
		params.Output,
	}

	log.Debugf("Executing hermeto %s", strings.Join(args, " "))
	_, _, _, err := hc.Executor.ExecuteWithOutput("hermeto", args...)
	return err
}

type InjectFilesParams struct {
	OutputDir    string
	ForOutputDir string
}

// Run the Hermeto inject-files command.
func (hc *HermetoCli) InjectFiles(params *InjectFilesParams) error {
	logLevel := logger.Logger.GetLevel().String()

	args := []string{
		"--log-level",
		logLevel,
		"inject-files",
		params.OutputDir,
		"--for-output-dir",
		params.ForOutputDir,
	}

	log.Debugf("Executing hermeto %s", strings.Join(args, " "))
	_, _, _, err := hc.Executor.ExecuteWithOutput("hermeto", args...)
	return err
}
