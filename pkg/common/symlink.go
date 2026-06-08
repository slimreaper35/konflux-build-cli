package common

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

// CheckSymlinks walks the given directory tree and returns an error if any
// symlink points to a target outside the directory.
// excludePatterns are path patterns matched against each symlink's path (not its target),
// relative to dir; patterns must not start with '/'. Matching symlinks are skipped entirely.
// Patterns use * and ? wildcards (* matches across '/').
func CheckSymlinks(dir string, excludePatterns []string) error {
	l.Logger.Debugf("Checking for symlinks pointing outside the directory %s", dir)

	baseDir, err := ResolvePath(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve directory path: %w", err)
	}

	checkoutRoot := filepath.Clean(dir)
	compiledExcludes, err := compileExcludePatterns(excludePatterns)
	if err != nil {
		return err
	}

	var invalidSymlinks []string

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Type()&os.ModeSymlink != 0 {
			cleanPath := filepath.Clean(path)
			if symlinkPathExcluded(cleanPath, checkoutRoot, compiledExcludes) {
				l.Logger.Debugf("Skipping symlink check for excluded path: %s", cleanPath)
				return nil
			}

			resolvedTarget, err := ResolvePath(path)
			if err != nil {
				l.Logger.Errorf("Broken symlink found: %s", path)
				invalidSymlinks = append(invalidSymlinks, path)
				return nil //nolint:nilerr
			}

			if !resolvedTarget.IsRelativeTo(baseDir) {
				l.Logger.Errorf("Symlink points outside directory: %s -> %s", path, resolvedTarget)
				invalidSymlinks = append(invalidSymlinks, path)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	if len(invalidSymlinks) > 0 {
		return fmt.Errorf("found %d symlink(s) pointing outside the directory", len(invalidSymlinks))
	}

	l.Logger.Debug("Symlink check passed")
	return nil
}

// compileExcludePatterns validates and pre-compiles exclusion patterns.
func compileExcludePatterns(patterns []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.HasPrefix(pattern, "/") || filepath.IsAbs(pattern) {
			return nil, fmt.Errorf("exclusion pattern %q must be relative to the checkout directory (must not start with '/')", pattern)
		}
		re, err := pathPatternToRegexp(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid exclusion pattern %q: %w", pattern, err)
		}
		out = append(out, re)
	}
	return out, nil
}

func symlinkPathExcluded(path, checkoutRoot string, patterns []*regexp.Regexp) bool {
	rel, err := filepath.Rel(checkoutRoot, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, re := range patterns {
		if re.MatchString(rel) {
			return true
		}
	}
	return false
}

// pathPatternToRegexp converts a path pattern to an anchored regexp.
// Wildcards: * matches any run of characters including '/';
// ? matches any single character. Other characters are literal.
func pathPatternToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '.', '+', '(', ')', '|', '^', '$', '[', ']', '{', '}', '\\':
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
		default:
			b.WriteByte(pattern[i])
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
