package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustExternalSymlink creates a symlink at checkoutDir/relLink pointing outside checkoutDir.
func mustExternalSymlink(t *testing.T, checkoutDir, relLink string) {
	t.Helper()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(checkoutDir, relLink)
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
}

func Test_pathPatternToRegexp(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		match   []string
		noMatch []string
	}{
		{
			name:    "literal dot",
			pattern: "foo.bar",
			match:   []string{"foo.bar"},
			noMatch: []string{"fooXbar"},
		},
		{
			name:    "star matches including slash",
			pattern: "pre*post",
			match:   []string{"prepost", "pre/post", "pre/a/b/post"},
			noMatch: []string{"pre", "post", "xprepost"},
		},
		{
			name:    "question mark matches one character",
			pattern: "a?c",
			match:   []string{"abc", "a1c", "a/c"},
			noMatch: []string{"ac", "abbc"},
		},
		{
			name:    "literal plus",
			pattern: "a+b",
			match:   []string{"a+b"},
			noMatch: []string{"ab", "aaab"},
		},
		{
			name:    "literal parentheses",
			pattern: "pkg(name)",
			match:   []string{"pkg(name)"},
			noMatch: []string{"pkgname", "pkgxnamey"},
		},
		{
			name:    "literal pipe",
			pattern: "a|b",
			match:   []string{"a|b"},
			noMatch: []string{"a", "b"},
		},
		{
			name:    "literal caret and dollar",
			pattern: "^foo$",
			match:   []string{"^foo$"},
			noMatch: []string{"foo", "xfoo"},
		},
		{
			name:    "literal brackets",
			pattern: "a[b]c",
			match:   []string{"a[b]c"},
			noMatch: []string{"abc", "aac"},
		},
		{
			name:    "literal braces",
			pattern: "a{b}c",
			match:   []string{"a{b}c"},
			noMatch: []string{"abc", "ac"},
		},
		{
			name:    "literal backslash",
			pattern: `a\b`,
			match:   []string{`a\b`},
			noMatch: []string{`aab`, `ab`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re, err := pathPatternToRegexp(tc.pattern)
			if err != nil {
				t.Fatalf("pathPatternToRegexp(%q): %v", tc.pattern, err)
			}
			for _, s := range tc.match {
				if !re.MatchString(s) {
					t.Errorf("pattern %q: expected %q to match", tc.pattern, s)
				}
			}
			for _, s := range tc.noMatch {
				if re.MatchString(s) {
					t.Errorf("pattern %q: expected %q not to match", tc.pattern, s)
				}
			}
		})
	}
}

func TestCheckSymlinks(t *testing.T) {
	t.Run("fails on external symlink without exclusions", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, "link")

		if err := CheckSymlinks(dir, nil); err == nil {
			t.Fatal("expected error for external symlink")
		}
	})

	t.Run("passes when external symlink path is excluded", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, "link")

		if err := CheckSymlinks(dir, []string{"link"}); err != nil {
			t.Fatalf("expected pass with exact exclusion: %v", err)
		}
	})

	t.Run("passes when broken symlink path is excluded", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(dir, "broken")
		if err := os.Symlink(filepath.Join(dir, "missing-target"), link); err != nil {
			t.Fatal(err)
		}

		if err := CheckSymlinks(dir, []string{"broken"}); err != nil {
			t.Fatalf("expected pass for excluded broken symlink: %v", err)
		}
	})

	t.Run("fails on broken symlink when not excluded", func(t *testing.T) {
		dir := t.TempDir()
		link := filepath.Join(dir, "broken")
		if err := os.Symlink(filepath.Join(dir, "missing-target"), link); err != nil {
			t.Fatal(err)
		}

		if err := CheckSymlinks(dir, nil); err == nil {
			t.Fatal("expected error for broken symlink")
		}
	})

	t.Run("partial exclusion still fails", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, filepath.Join("keep", "ok"))
		mustExternalSymlink(t, dir, "bad")

		if err := CheckSymlinks(dir, []string{"keep/*"}); err == nil {
			t.Fatal("expected error when only one external symlink is excluded")
		}
	})

	t.Run("trailing wildcard exclusion", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, filepath.Join("vendor", "link"))

		if err := CheckSymlinks(dir, []string{"vendor/*"}); err != nil {
			t.Fatalf("expected pass with trailing wildcard: %v", err)
		}
	})

	t.Run("leading wildcard exclusion", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, filepath.Join("vendor", "link"))

		if err := CheckSymlinks(dir, []string{"*/link"}); err != nil {
			t.Fatalf("expected pass with leading wildcard: %v", err)
		}
	})

	t.Run("embedded wildcard exclusion", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, filepath.Join("nested", "vendor", "link"))

		if err := CheckSymlinks(dir, []string{"*/vendor/*"}); err != nil {
			t.Fatalf("expected pass with embedded wildcard: %v", err)
		}
	})

	t.Run("non-matching exclusion pattern still fails", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, filepath.Join("vendor", "link"))

		if err := CheckSymlinks(dir, []string{"other/*"}); err == nil {
			t.Fatal("expected error when exclusion pattern does not match symlink path")
		}
	})

	t.Run("space inside pattern is significant", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, filepath.Join("vendor", "link"))

		if err := CheckSymlinks(dir, []string{"vendor/li k"}); err == nil {
			t.Fatal("expected error when pattern space prevents match")
		}
	})

	t.Run("rejects absolute exclusion pattern", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, "link")

		err := CheckSymlinks(dir, []string{"/vendor/*"})
		if err == nil {
			t.Fatal("expected error for absolute exclusion pattern")
		}
		if !strings.Contains(err.Error(), "must not start with '/'") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("question mark wildcard exclusion", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, "a1")

		if err := CheckSymlinks(dir, []string{"a?"}); err != nil {
			t.Fatalf("expected pass with ? wildcard: %v", err)
		}
	})

	t.Run("trims and skips empty patterns", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, "link")

		if err := CheckSymlinks(dir, []string{"", "  ", " link "}); err != nil {
			t.Fatalf("expected pass when only non-empty trimmed pattern matches: %v", err)
		}
	})

	t.Run("in-tree path starting with dot-dot is not treated as outside checkout", func(t *testing.T) {
		dir := t.TempDir()
		mustExternalSymlink(t, dir, filepath.Join("..foo", "link"))

		if err := CheckSymlinks(dir, []string{"..foo/*"}); err != nil {
			t.Fatalf("expected exclusion to match ..foo path inside checkout: %v", err)
		}
	})
}
