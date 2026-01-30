package prefetch_dependencies

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

const readOnlyFileMode = os.FileMode(0444)

// These environment variables are defined in the prefetch-dependencies Tekton task.
const (
	envWorkspaceGitBasicAuthBound = "WORKSPACE_GIT_BASIC_AUTH_BOUND"
	envWorkspaceGitBasicAuthPath  = "WORKSPACE_GIT_BASIC_AUTH_PATH"
	envWorkspaceNetrcBound        = "WORKSPACE_NETRC_BOUND"
	envWorkspaceNetrcPath         = "WORKSPACE_NETRC_PATH"
)

// These volume mounts are defined in the prefetch-dependencies Tekton task.
const (
	volumeMount1 = "/mnt/trusted-ca"
	volumeMount2 = "/activation-key"
)

const (
	caBundlePath          = volumeMount1 + "/ca-bundle.crt"
	rhsmOrgPath           = volumeMount2 + "/org"
	rhsmActivationKeyPath = volumeMount2 + "/activationkey"
)

func renameRepoFiles(outputDir string) error {
	var repoFiles []string

	log.Debugf("Searching for repo files in %s", outputDir)
	err := filepath.WalkDir(outputDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Hermeto will create a hermeto.repo file when processing RPMs (one for each architecture).
		if !entry.IsDir() && filepath.Base(path) == "hermeto.repo" {
			repoFiles = append(repoFiles, path)
		}
		return nil
	})

	if err != nil {
		return err
	}

	for _, repoFile := range repoFiles {
		// TODO: Change cachi2.repo to more suitable name like prefetch.repo.
		newRepoFile := filepath.Join(filepath.Dir(repoFile), "cachi2.repo")
		if err := os.Rename(repoFile, newRepoFile); err != nil {
			return err
		}
		log.Debugf("Successfully renamed %s to %s", repoFile, newRepoFile)
	}

	if len(repoFiles) == 0 {
		log.Debug("No repo files found")
	}
	return nil
}

func parseInput(input string) any {
	var result any
	if err := json.Unmarshal([]byte(input), &result); err != nil {
		// Input is not valid JSON => assume it is a string.
		return map[string]any{"type": input}
	}

	// Input is valid JSON => return it as is.
	return result
}

// Check if the user input contains an RPM package.
func containsRPM(input any) bool {
	switch data := input.(type) {
	case []any:
		if slices.ContainsFunc(data, containsRPM) {
			return true
		}

	case map[string]any:
		if packages, ok := data["packages"].([]any); ok {
			if slices.ContainsFunc(packages, containsRPM) {
				return true
			}
		}

		if typeValue, ok := data["type"]; ok && typeValue == "rpm" {
			return true
		}
	}
	return false
}

func injectRPMInput(input any) (any, error) {
	withSummary := injectSummaryInSBOMField(input)

	if !fileExists(rhsmOrgPath) {
		// Do not inject SSL options if the RHSM organization does not exist.
		log.Info("No RHSM organization found, skipping SSL configuration")
		return withSummary, nil
	}

	if err := registerSubscriptionManager(); err != nil {
		return nil, err
	}

	// Glob ignores file system errors such as I/O errors reading directories.
	// The only possible returned error is ErrBadPattern, when pattern is malformed.
	entitlementFiles, _ := filepath.Glob("/etc/pki/entitlement/*.pem")

	var clientKeyPath, clientCertPath string
	for _, file := range entitlementFiles {
		if strings.HasSuffix(file, "-key.pem") {
			clientKeyPath = file
		} else {
			clientCertPath = file
		}
	}

	if clientKeyPath == "" || clientCertPath == "" {
		return nil, errors.New("no entitlement certificates found")
	}

	ssl := map[string]any{
		"client_key":  clientKeyPath,
		"client_cert": clientCertPath,
		"ca_bundle":   "/etc/rhsm/ca/redhat-uep.pem",
	}
	return injectSSLOptions(withSummary, ssl), nil
}

func injectSummaryInSBOMField(input any) any {
	switch data := input.(type) {
	case []any:
		// Array format: [{"type": "rpm"}]
		for i, item := range data {
			data[i] = injectSummaryInSBOMField(item)
		}
		return data

	case map[string]any:
		// Object format with "packages" field: {"packages": {"type": "rpm"}]}
		if packages, ok := data["packages"].([]any); ok {
			for i, item := range packages {
				packages[i] = injectSummaryInSBOMField(item)
			}
			return data
		}

		// Object format with "type" field: {"type": "rpm"}
		if typeValue, ok := data["type"]; ok && typeValue == "rpm" {
			data["include_summary_in_sbom"] = true
			return data
		}
	}
	return input
}

func injectSSLOptions(input any, ssl any) any {
	switch data := input.(type) {
	case []any:
		// Array format: [{"type": "rpm"}]
		for i, item := range data {
			data[i] = injectSSLOptions(item, ssl)
		}
		return data

	case map[string]any:
		// Object format with "packages" field: {"packages": [{"type": "rpm"}]}
		if packages, ok := data["packages"].([]any); ok {
			for i, item := range packages {
				packages[i] = injectSSLOptions(item, ssl)
			}
			return data
		}

		// Object format with "type" field: {"type": "rpm"}
		if typeValue, ok := data["type"]; ok && typeValue == "rpm" {
			if existingOptions, ok := data["options"].(map[string]any); ok {
				if existingSSL, ok := existingOptions["ssl"].(map[string]any); ok {
					maps.Copy(existingSSL, ssl.(map[string]any))
					return data
				}
			}
			data["options"] = map[string]any{"ssl": ssl}
			return data
		}
	}
	return input
}

func registerSubscriptionManager() error {
	org, err := os.ReadFile(rhsmOrgPath)
	if err != nil {
		return err
	}

	activationKey, err := os.ReadFile(rhsmActivationKeyPath)
	if err != nil {
		return err
	}

	args := []string{
		"register",
		"--force",
		"--org", string(org),
		"--activationkey", string(activationKey),
	}

	executor := cliwrappers.NewCliExecutor()
	command := func() (stdout string, stderr string, errCode int, err error) {
		return executor.Execute("subscription-manager", args...)
	}

	retryer := cliwrappers.NewRetryer(command).StopIfOutputContains("unauthorized")
	_, _, _, err = retryer.Run()
	if err == nil {
		log.Info("Successfully registered with subscription-manager")
	}
	return err
}

func yoloUnregisterSubscriptionManager() {
	executor := cliwrappers.NewCliExecutor()
	executor.Execute("subscription-manager", "unregister")
}

func updateTrustStore() error {
	if !fileExists(caBundlePath) {
		log.Warn("No trusted CA bundle found, skipping trust store update")
		return nil
	}

	// Copy CA bundle into anchors directory.
	destination := filepath.Join("/etc/pki/ca-trust/source/anchors", filepath.Base(caBundlePath))
	if err := cpFile(caBundlePath, destination); err != nil {
		return err
	}

	executor := cliwrappers.NewCliExecutor()
	_, _, _, err := executor.Execute("update-ca-trust")
	return err
}

func cpFile(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	return os.WriteFile(destinationPath, data, readOnlyFileMode)
}

func copyNetrcFile() error {
	if os.Getenv(envWorkspaceNetrcBound) != "true" {
		return nil
	}

	workspace := os.Getenv(envWorkspaceNetrcPath)
	home := os.Getenv("HOME")
	return cpFile(filepath.Join(workspace, ".netrc"), filepath.Join(home, ".netrc"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil && info.Mode().IsRegular()
}

// TODO: This could be shared with the git-clone command.
func setupGitBasicAuth(sourceDir string) error {
	if os.Getenv(envWorkspaceGitBasicAuthBound) != "true" {
		return nil
	}

	authDir := os.Getenv(envWorkspaceGitBasicAuthPath)
	homeDir := os.Getenv("HOME")

	gitCredentialsPath := filepath.Join(authDir, ".git-credentials")
	gitConfigPath := filepath.Join(authDir, ".gitconfig")

	if fileExists(gitCredentialsPath) && fileExists(gitConfigPath) {
		if err := cpFile(gitCredentialsPath, filepath.Join(homeDir, ".git-credentials")); err != nil {
			return err
		}
		if err := cpFile(gitConfigPath, filepath.Join(homeDir, ".gitconfig")); err != nil {
			return err
		}
		return nil
	}

	usernamePath := filepath.Join(authDir, "username")
	passwordPath := filepath.Join(authDir, "password")

	if fileExists(usernamePath) && fileExists(passwordPath) {
		rawUsername, err := os.ReadFile(usernamePath)
		if err != nil {
			return err
		}
		rawPassword, err := os.ReadFile(passwordPath)
		if err != nil {
			return err
		}

		username := strings.TrimSpace(string(rawUsername))
		password := strings.TrimSpace(string(rawPassword))
		hostname, err := getHostnameFromRemoteOriginURL(sourceDir)
		if err != nil {
			return err
		}

		gitCredentialsContent := fmt.Sprintf("https://%s:%s@%s", username, password, hostname)
		if err := os.WriteFile(filepath.Join(homeDir, ".git-credentials"), []byte(gitCredentialsContent), readOnlyFileMode); err != nil {
			return err
		}
		gitConfigContent := fmt.Sprintf("[credential \"https://%s\"]\nhelper = store", hostname)
		if err := os.WriteFile(filepath.Join(homeDir, ".gitconfig"), []byte(gitConfigContent), readOnlyFileMode); err != nil {
			return err
		}

		return nil
	}

	return errors.New("unknown git basic auth workspace format")
}

func getHostnameFromRemoteOriginURL(sourceDir string) (string, error) {
	executor := cliwrappers.NewCliExecutor()
	stdout, _, _, err := executor.ExecuteInDir(sourceDir, "git", "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}

	parsedURL, err := url.Parse(strings.TrimSpace(stdout))
	if err != nil {
		return "", err
	}

	return parsedURL.Hostname(), nil
}

func dropGoProxyFrom(configFileContent string) string {
	var modifiedConfigFileContent []string

	inGomodBlock := false
	lines := strings.Split(configFileContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Remove the deprecated top-level goproxy_url.
		if strings.HasPrefix(trimmed, "goproxy_url:") {
			continue
		}

		if strings.HasPrefix(trimmed, "gomod:") {
			inGomodBlock = true
			modifiedConfigFileContent = append(modifiedConfigFileContent, line)
			continue
		}

		// Remove the proxy_url inside the gomod block.
		if inGomodBlock {
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inGomodBlock = false
			} else if strings.HasPrefix(trimmed, "proxy_url:") {
				continue
			}
		}
		modifiedConfigFileContent = append(modifiedConfigFileContent, line)
	}

	result := strings.Join(modifiedConfigFileContent, "\n")
	log.Debugf("Using modified config file content for Hermeto:\n%s", result)
	return result
}
