package prefetch_dependencies

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

const ReadOnlyFileMode = os.FileMode(0444)

const (
	rhsmOrgPath           = "/activation-key/org"
	rhsmActivationKeyPath = "/activation-key/activationkey"
	rhsmCABundlePath      = "/etc/rhsm/ca/redhat-uep.pem"
)

func RenameRepoFiles(outputDir string) error {
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
		log.Debugf("Successfully renamed %s -> %s", repoFile, newRepoFile)
	}

	return nil
}

func ParseInput(input string) any {
	var result any
	if err := json.Unmarshal([]byte(input), &result); err != nil {
		// Input is not valid JSON => assume it is a string.
		return map[string]any{"type": input}
	}

	// Input is valid JSON => return it as is.
	return result
}

// Check if the user input contains an RPM package.
func ContainsRPM(input any) bool {
	switch data := input.(type) {
	case []any:
		if slices.ContainsFunc(data, ContainsRPM) {
			return true
		}

	case map[string]any:
		if packages, ok := data["packages"].([]any); ok {
			if slices.ContainsFunc(packages, ContainsRPM) {
				return true
			}
		}

		if typeValue, ok := data["type"]; ok && typeValue == "rpm" {
			return true
		}
	}
	return false
}

func InjectRPMInput(input any) (any, error) {
	withSummary := injectSummaryInSBOMField(input)

	if _, err := os.Stat(rhsmOrgPath); err != nil {
		// Do not inject SSL options if the RHSM organization does not exist.
		log.Info("No RHSM organization found, skipping SSL configuration")
		return withSummary, nil
	}

	if err := RegisterSubscriptionManager(); err != nil {
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
		"ca_bundle":   rhsmCABundlePath,
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

// TODO: This could be shared with the build command.
func RegisterSubscriptionManager() error {
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
	return err
}

// TODO: This could be shared with the build command.
func YoloUnregisterSubscriptionManager() {
	executor := cliwrappers.NewCliExecutor()
	executor.Execute("subscription-manager", "unregister")
}

func UpdateTrustStore() error {
	sourcePath := "/mnt/trusted-ca/ca-bundle.crt"
	destinationPath := "/etc/pki/ca-trust/source/anchors"

	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		log.Warn("No trusted CA bundle found, skipping trust store update")
		return nil
	}

	if err := cpFiles(sourcePath, destinationPath); err != nil {
		return err
	}

	executor := cliwrappers.NewCliExecutor()
	_, _, _, err := executor.Execute("update-ca-trust")
	return err
}

func cpFiles(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return err
	}

	destination, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	if err := os.Chmod(destinationPath, ReadOnlyFileMode); err != nil {
		return err
	}

	log.Debugf("Successfully copied %s -> %s", sourcePath, destinationPath)
	return nil
}

func CopyNetrcFile() error {
	if os.Getenv("WORKSPACE_NETRC_BOUND") == "true" {
		workspace := os.Getenv("WORKSPACE_NETRC_PATH")
		home := os.Getenv("HOME")
		if err := cpFiles(filepath.Join(workspace, ".netrc"), filepath.Join(home, ".netrc")); err != nil {
			return err
		}
	}
	return nil
}

func ConfigureAuthenticationForGit(sourceDir string) error {
	if os.Getenv("WORKSPACE_GIT_BASIC_AUTH_BOUND") != "true" {
		// Skip authentication configuration.
		return nil
	}

	if err := copyGitAuthFiles(); err != nil {
		return err
	}

	if err := generateGitCredentials(sourceDir); err != nil {
		return err
	}

	return nil
}

// TODO: This could be shared with the git-clone command.
func copyGitAuthFiles() error {
	workspace := os.Getenv("WORKSPACE_GIT_BASIC_AUTH_PATH")
	home := os.Getenv("HOME")

	if err := cpFiles(filepath.Join(workspace, ".git-credentials"), filepath.Join(home, ".git-credentials")); err != nil {
		return err
	}

	if err := cpFiles(filepath.Join(workspace, ".gitconfig"), filepath.Join(home, ".gitconfig")); err != nil {
		return err
	}

	return nil
}

// TODO: This could be shared with the git-clone command.
func generateGitCredentials(sourceDir string) error {
	hostname, err := getHostnameFromRemoteOriginURL(sourceDir)
	if err != nil {
		return err
	}

	workspace := os.Getenv("WORKSPACE_GIT_BASIC_AUTH_PATH")
	home := os.Getenv("HOME")

	rawUsername, err := os.ReadFile(filepath.Join(workspace, "username"))
	if err != nil {
		return err
	}
	rawPassword, err := os.ReadFile(filepath.Join(workspace, "password"))
	if err != nil {
		return err
	}

	username := strings.TrimSpace(string(rawUsername))
	password := strings.TrimSpace(string(rawPassword))

	gitCredentialsContent := fmt.Sprintf("https://%s:%s@%s", username, password, hostname)
	gitCredentialsPath := filepath.Join(home, ".git-credentials")
	if err := os.WriteFile(gitCredentialsPath, []byte(gitCredentialsContent), ReadOnlyFileMode); err != nil {
		return err
	}

	gitConfigContent := fmt.Sprintf("[credential \"https://%s\"]\nhelper = store", hostname)
	gitConfigPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitConfigPath, []byte(gitConfigContent), ReadOnlyFileMode); err != nil {
		return err
	}

	return nil
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

// TODO: Drop this once https://github.com/hermetoproject/hermeto/issues/577 is resolved.
func DropGoProxyFrom(configFileContent string) string {
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

	log.Debugf("Using modified config file content for Hermeto:\n%s", modifiedConfigFileContent)
	return strings.Join(modifiedConfigFileContent, "\n")
}
