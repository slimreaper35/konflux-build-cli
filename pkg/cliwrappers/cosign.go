package cliwrappers

import (
	"fmt"
	"os"

	"github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

var cosignLog = logger.Logger.WithField("logger", "CosignCli")

type CosignCli struct {
	Executor CliExecutorInterface
}

func NewCosignCli(executor CliExecutorInterface) (*CosignCli, error) {
	cliAvailable, err := CheckCliToolAvailable("cosign")
	if err != nil {
		return nil, err
	}
	if !cliAvailable {
		return nil, fmt.Errorf("cosign CLI is not available")
	}

	return &CosignCli{Executor: executor}, nil
}

type CosignSignArgs struct {
	Image         string
	RekorURL      string
	FulcioURL     string
	OIDCIssuer    string
	OIDCTokenPath string
}

func (cc *CosignCli) Sign(args *CosignSignArgs, dockerConfigDir string) error {
	cmdArgs := []string{
		"sign",
		args.Image,
		"--yes",
		// Specifying service URLs is deprecated.
		// To use them, we need to explicitly disable the signing config file.
		"--use-signing-config=false",
	}

	if args.RekorURL != "" {
		cmdArgs = append(cmdArgs, "--rekor-url", args.RekorURL)
	}
	if args.FulcioURL != "" {
		cmdArgs = append(cmdArgs, "--fulcio-url", args.FulcioURL)
	}
	if args.OIDCIssuer != "" {
		cmdArgs = append(cmdArgs, "--oidc-issuer", args.OIDCIssuer)
	}

	var cmdEnv []string
	if args.OIDCTokenPath != "" {
		oidcToken, err := os.ReadFile(args.OIDCTokenPath)
		if err != nil {
			return err
		}
		cmdEnv = append(cmdEnv, "SIGSTORE_ID_TOKEN="+string(oidcToken))
	}
	if dockerConfigDir != "" {
		cmdEnv = append(cmdEnv, "DOCKER_CONFIG="+dockerConfigDir)
	}

	cmd := Cmd{Name: "cosign", Args: cmdArgs, Env: cmdEnv, LogOutput: true}

	cosignLog.Debugf("Running command:\n%s", shellJoin(cmd.Name, cmd.Args...))
	retryer := NewRetryer(func() (string, string, int, error) {
		return cc.Executor.Execute(cmd)
	})

	_, stderr, _, err := retryer.Run()
	if err != nil {
		cosignLog.Errorf("cosign sign failed: %s", err.Error())
		if stderr != "" {
			cosignLog.Errorf("[stderr]\n%s", stderr)
		}
		return err
	}
	return nil
}
