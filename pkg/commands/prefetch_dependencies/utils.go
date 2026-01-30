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

const (
	envWorkspaceGitBasicAuthBound = "WORKSPACE_GIT_BASIC_AUTH_BOUND"
	envWorkspaceGitBasicAuthPath  = "WORKSPACE_GIT_BASIC_AUTH_PATH"
	envWorkspaceNetrcBound        = "WORKSPACE_NETRC_BOUND"
	envWorkspaceNetrcPath         = "WORKSPACE_NETRC_PATH"
)

// Rename repo files in the output directory to expected cachi2.repo.
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
		// TODO: Change cachi2.repo to a more generic name like prefetch.repo or do not rename at all.
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

// Parse the user input to a valid JSON object.
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

// Modify the user input for RPM packages.
func injectRPMInput(input any, rhsmOrg string, rhsmActivationKey string) (any, error) {
	withSummary := injectSummaryInSBOMField(input)

	if !fileExists(rhsmOrg) || !fileExists(rhsmActivationKey) {
		return withSummary, nil
	}

	if err := registerSubscriptionManager(rhsmOrg, rhsmActivationKey); err != nil {
		return withSummary, err
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
		return nil, errors.New("no entitlement certificate files found")
	}

	ssl := map[string]any{
		"client_key":  clientKeyPath,
		"client_cert": clientCertPath,
		"ca_bundle":   "/etc/rhsm/ca/redhat-uep.pem",
	}
	return injectSSLOptions(withSummary, ssl), nil
}

// Inject a flag to enable RPM summary in the SBOM.
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

// Inject SSL options for all RPM packages.
func injectSSLOptions(input any, ssl map[string]any) any {
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
					maps.Copy(existingSSL, ssl)
					return data
				}
			}
			data["options"] = map[string]any{"ssl": ssl}
			return data
		}
	}
	return input
}

// Wrapper around the subscription-manager register command.
func registerSubscriptionManager(rhsmOrg string, rhsmActivationKey string) error {
	available, err := cliwrappers.CheckCliToolAvailable("subscription-manager")
	if err != nil {
		return err
	}
	if !available {
		return errors.New("subscription-manager CLI is not available")
	}

	org, err := os.ReadFile(rhsmOrg)
	if err != nil {
		return err
	}

	key, err := os.ReadFile(rhsmActivationKey)
	if err != nil {
		return err
	}

	args := []string{
		"register",
		"--force",
		"--org", string(org),
		"--activationkey", string(key),
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

// Wrapper around the subscription-manager unregister command.
func unregisterSubscriptionManager() {
	executor := cliwrappers.NewCliExecutor()
	executor.Execute("subscription-manager", "unregister")
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

// Copy the .netrc file from the workspace to the home directory.
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
// Copy git credentials and config files from the workspace to the home directory.
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

// Parse the hostname from the git remote origin URL.
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

// Temporarily drop Go proxy URL from the config file content.
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
	log.Debugf("Using modified config file content:\n%s", result)
	return result
}
