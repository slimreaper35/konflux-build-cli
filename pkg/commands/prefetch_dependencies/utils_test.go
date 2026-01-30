package prefetch_dependencies

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	. "github.com/onsi/gomega"
)

func TestRenameRepoFiles(t *testing.T) {
	g := NewWithT(t)

	const exampleContent = `
	[repo]
	name=test
	baseurl=https://test.com
	enabled=1
	`

	t.Run("should succeed if no repo files are found", func(t *testing.T) {
		tempDir := t.TempDir()

		g.Expect(RenameRepoFiles(tempDir)).To(Succeed())

		entries, _ := os.ReadDir(tempDir)
		g.Expect(entries).To(BeEmpty())
	})

	t.Run("should rename one repo file", func(t *testing.T) {
		tempDir := t.TempDir()
		repoFile := filepath.Join(tempDir, "hermeto.repo")

		g.Expect(os.WriteFile(repoFile, []byte(exampleContent), 0644)).To(Succeed())

		g.Expect(RenameRepoFiles(tempDir)).To(Succeed())

		expectedRepoFile := filepath.Join(tempDir, "cachi2.repo")
		content, _ := os.ReadFile(expectedRepoFile)
		g.Expect(string(content)).To(Equal(exampleContent))

		g.Expect(repoFile).ToNot(BeAnExistingFile())
	})

	t.Run("should rename multiple repo files", func(t *testing.T) {
		tempDir := t.TempDir()
		archs := []string{"aarch64", "x86_64", "arm64"}
		for _, arch := range archs {
			g.Expect(os.MkdirAll(filepath.Join(tempDir, arch), 0755)).To(Succeed())
			g.Expect(os.WriteFile(filepath.Join(tempDir, arch, "hermeto.repo"), []byte(exampleContent), 0644)).To(Succeed())
		}

		g.Expect(RenameRepoFiles(tempDir)).To(Succeed())

		for _, arch := range archs {
			expectedRepoFile := filepath.Join(tempDir, arch, "cachi2.repo")
			content, _ := os.ReadFile(expectedRepoFile)
			g.Expect(string(content)).To(Equal(exampleContent))
		}

		for _, arch := range archs {
			repoFile := filepath.Join(tempDir, arch, "hermeto.repo")
			g.Expect(repoFile).ToNot(BeAnExistingFile())
		}
	})
}

func TestParseInput(t *testing.T) {
	g := NewWithT(t)

	t.Run("should parse input JSON object", func(t *testing.T) {
		json := `{"foo": "bar"}`
		data := ParseInput(json)
		g.Expect(data).To(Equal(map[string]any{"foo": "bar"}))
	})

	t.Run("should parse input JSON array", func(t *testing.T) {
		json := `[{"foo": "bar"}, {"foo": "baz"}]`
		data := ParseInput(json)
		g.Expect(data).To(Equal([]any{map[string]any{"foo": "bar"}, map[string]any{"foo": "baz"}}))
	})

	t.Run("should parse input JSON object with packages array", func(t *testing.T) {
		input := `{"packages": [{"foo": "bar"}, {"foo": "baz"}]}`
		data := ParseInput(input)
		g.Expect(data).To(Equal(map[string]any{"packages": []any{map[string]any{"foo": "bar"}, map[string]any{"foo": "baz"}}}))
	})

	t.Run("should convert plain string to JSON object", func(t *testing.T) {
		input := "foo"
		data := ParseInput(input)
		g.Expect(data).To(Equal(map[string]any{"type": "foo"}))
	})
}

func TestContainsRPM(t *testing.T) {
	g := NewWithT(t)

	t.Run("should return true", func(t *testing.T) {
		input := `{"type": "rpm"}`
		data := ParseInput(input)
		g.Expect(ContainsRPM(data)).To(BeTrue())
	})

	t.Run("should return false", func(t *testing.T) {
		input := `{"type": "yarn"}`
		data := ParseInput(input)
		g.Expect(ContainsRPM(data)).To(BeFalse())
	})

	t.Run("should return true", func(t *testing.T) {
		input := `{"packages": [{"type": "rpm"}, {"type": "yarn"}]}`
		data := ParseInput(input)
		g.Expect(ContainsRPM(data)).To(BeTrue())
	})

	t.Run("should return true", func(t *testing.T) {
		input := `[{"type": "rpm"}, {"type": "yarn"}]`
		data := ParseInput(input)
		g.Expect(ContainsRPM(data)).To(BeTrue())
	})
}

func TestInjectSummaryInSBOMField(t *testing.T) {
	g := NewWithT(t)

	t.Run("should inject summary in SBOM field for an RPM package", func(t *testing.T) {
		input := `{"type": "rpm"}`
		data := ParseInput(input)
		g.Expect(injectSummaryInSBOMField(data)).To(Equal(map[string]any{"type": "rpm", "include_summary_in_sbom": true}))
	})
}

func TestInjectSSLOptions(t *testing.T) {
	g := NewWithT(t)

	exampleSSLOptions := map[string]any{
		"client_key":  "client_key",
		"client_cert": "client_cert",
		"ca_bundle":   "ca_bundle",
	}

	t.Run("should inject SSL options for RPM package", func(t *testing.T) {
		input := `{"type": "rpm"}`
		data := ParseInput(input)
		g.Expect(injectSSLOptions(data, exampleSSLOptions)).To(Equal(map[string]any{"type": "rpm", "options": map[string]any{"ssl": exampleSSLOptions}}))
	})

	t.Run("should merge SSL options if they already exist for RPM package", func(t *testing.T) {
		input := `{"type": "rpm", "options": {"ssl": {"client_key": "my_client_key"}}}`
		data := ParseInput(input)
		g.Expect(injectSSLOptions(data, exampleSSLOptions)).To(Equal(map[string]any{"type": "rpm", "options": map[string]any{"ssl": map[string]any{"client_key": "client_key", "client_cert": "client_cert", "ca_bundle": "ca_bundle"}}}))
	})
}

func TestGetHostnameFromRemoteOriginURL(t *testing.T) {
	g := NewWithT(t)

	t.Run("should get hostname from HTTPS origin URL", func(t *testing.T) {
		tempDir := t.TempDir()
		executor := cliwrappers.NewCliExecutor()

		_, _, _, err := executor.ExecuteInDir(tempDir, "git", "init")
		g.Expect(err).ToNot(HaveOccurred())
		_, _, _, err = executor.ExecuteInDir(tempDir, "git", "remote", "add", "origin", "https://github.com/user/repo.git")
		g.Expect(err).ToNot(HaveOccurred())

		hostname, err := getHostnameFromRemoteOriginURL(tempDir)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(hostname).To(Equal("github.com"))
	})
}

func TestDropGoProxyFromConfigFileContent(t *testing.T) {
	g := NewWithT(t)

	t.Run("should drop go proxy field from config file content", func(t *testing.T) {
		const originalContent = `
		gomod:
		  foo: bar
		  proxy_url: https://example.com
		`
		const expectedContent = `
		gomod:
		  foo: bar
		`

		result := DropGoProxyFrom(originalContent)
		g.Expect(result).To(Equal(expectedContent))
	})

	t.Run("should drop deprecated go proxy field from config file content", func(t *testing.T) {
		const originalContent = `
		foo: bar
		goproxy_url: https://example.com
		`
		const expectedContent = `
		foo: bar
		`

		result := DropGoProxyFrom(originalContent)
		g.Expect(result).To(Equal(expectedContent))
	})
}
