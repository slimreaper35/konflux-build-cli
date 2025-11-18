package common

import (
	"regexp"
	"strconv"
	"strings"
)

var imageDigestSuffixRegex = regexp.MustCompile(`@sha256:[a-fA-F0-9]{64}$`)
var imageTagSuffixRegex = regexp.MustCompile(`:[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$`)

// GetImageName trims tag and/or digest from given image reference.
func GetImageName(imageURL string) string {
	imageWithoutDigest := imageDigestSuffixRegex.ReplaceAllString(imageURL, "")
	image := imageTagSuffixRegex.ReplaceAllString(imageWithoutDigest, "")
	return image
}

var imagePartRegex = regexp.MustCompile("^[a-z0-9](?:[a-z0-9_.-]*[a-z0-9])?$")
var imageRegistryAddressAndPortRegex = regexp.MustCompile(`^([a-z0-9](?:[a-z0-9_.-]*[a-z0-9])?)(?::(\d+))?$`)

// isImageNameValid validates image name without tag and digest.
// Image name can contain lowercase letters and digits plus separators: dash, period, underscore, slash.
// Image name cannot start or end with a separator.
// Image name max length is 128 characters.
// It's not allowed to have double separator and triple underscore.
func IsImageNameValid(imageName string) bool {
	if imageName == "" {
		return false
	}
	if len(imageName) > 128 {
		return false
	}
	if strings.Contains(imageName, "___") ||
		strings.Contains(imageName, "//") ||
		strings.Contains(imageName, "..") ||
		strings.Contains(imageName, "--") ||
		strings.Contains(imageName, "_.") ||
		strings.Contains(imageName, "._") ||
		strings.Contains(imageName, "-.") ||
		strings.Contains(imageName, ".-") ||
		strings.Contains(imageName, "-_") ||
		strings.Contains(imageName, "_-") {
		return false
	}

	parts := strings.Split(imageName, "/")
	if len(parts) == 1 {
		return imagePartRegex.MatchString(parts[0])
	}
	// Multiple path parts.
	// Handle the first part differently, since it might have a port.
	match := imageRegistryAddressAndPortRegex.FindStringSubmatch(parts[0])
	if len(match) == 0 {
		return false
	}
	if len(match) > 1 {
		registryAddress := match[1]
		if !imagePartRegex.MatchString(registryAddress) {
			// It should never happen if both regexs are correct.
			// Still keeping the check to catch regex editing mistakes.
			return false
		}
	}
	if len(match) > 2 && match[2] != "" {
		portString := match[2]
		if port, err := strconv.Atoi(portString); err == nil {
			if port < 0 || port > 65535 {
				return false
			}
		} else {
			return false
		}
	}
	// Validate the rest of the path parts the usual way.
	for _, part := range parts[1:] {
		if !imagePartRegex.MatchString(part) {
			return false
		}
	}
	return true
}

var imageTagRegex = regexp.MustCompile("^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$")

// isTagValid validates image tag.
// Image tag can contain letters and digits plus underscore, period, dash.
// Image tag cannot start with period or dash.
// Image tag max length is 128 characters.
func IsImageTagValid(tagName string) bool {
	return imageTagRegex.MatchString(tagName)
}

var imageDigestRegex = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func IsImageDigestValid(digest string) bool {
	return imageDigestRegex.MatchString(digest)
}
