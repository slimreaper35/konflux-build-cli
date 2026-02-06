package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const (
	containerfileArtifactTagSuffix = ".containerfile"
	containerfileArtifactType      = "application/vnd.konflux.containerfile"
	containerfileContext           = "."

	// Max length of a tag - length of sha256 digest: 128 - 71
	// Refer to https://github.com/opencontainers/distribution-spec/blob/main/spec.md#pulling-manifests
	tagSuffixRegex = "^[a-zA-Z0-9._-]{1,57}$"
)

var PushContainerfileParamsConfig = map[string]common.Parameter{
	"image-url": {
		Name:       "image-url",
		ShortName:  "i",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_IMAGE_URL",
		TypeKind:   reflect.String,
		Usage:      "Binary image URL. Containerfile is pushed to the image repository where this binary image is.",
		Required:   true,
	},
	"image-digest": {
		Name:       "image-digest",
		ShortName:  "d",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_IMAGE_DIGEST",
		TypeKind:   reflect.String,
		Usage:      "Binary image digest, which is used to construct the tag of Containerfile image.",
		Required:   true,
	},
	"containerfile": {
		Name:       "containerfile",
		ShortName:  "f",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_CONTAINERFILE",
		TypeKind:   reflect.String,
		Usage:      "Path to Containerfile relative to source repository root. If not specified, Containerfile is searched from context then the source directory. Fallback to search Dockerfile if no Containerfile is found.",
		Required:   false,
	},
	"context": {
		Name:         "context",
		ShortName:    "c",
		EnvVarName:   "KBC_PUSH_CONTAINERFILE_CONTEXT",
		TypeKind:     reflect.String,
		DefaultValue: containerfileContext,
		Usage:        "Build context used to search Containerfile.",
		Required:     false,
	},
	"tag-suffix": {
		Name:         "tag-suffix",
		ShortName:    "t",
		EnvVarName:   "KBC_PUSH_CONTAINERFILE_TAG_SUFFIX",
		TypeKind:     reflect.String,
		DefaultValue: containerfileArtifactTagSuffix,
		Usage:        "Suffix to construct artifact image tag",
		Required:     false,
	},
	"artifact-type": {
		Name:         "artifact-type",
		ShortName:    "a",
		EnvVarName:   "KBC_PUSH_CONTAINERFILE_ARTIFACT_TYPE",
		TypeKind:     reflect.String,
		DefaultValue: containerfileArtifactType,
		Usage:        "Artifact type of the Containerfile artifact image.",
		Required:     false,
	},
	"source": {
		Name:       "source",
		ShortName:  "s",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_SOURCE",
		TypeKind:   reflect.String,
		Usage:      "Directory containing the source code. It is a relative path to the root of current working directory.",
		Required:   true,
	},
	"result-path-image-ref": {
		Name:       "result-path-image-ref",
		ShortName:  "r",
		EnvVarName: "KBC_PUSH_CONTAINERFILE_RESULT_PATH_IMAGE_REF",
		TypeKind:   reflect.String,
		Usage:      "Write digested image reference of the pushed Containerfile image into this file.",
		Required:   false,
	},
}

type PushContainerfileParams struct {
	ImageUrl           string `paramName:"image-url"`
	ImageDigest        string `paramName:"image-digest"`
	Containerfile      string `paramName:"containerfile"`
	Context            string `paramName:"context"`
	TagSuffix          string `paramName:"tag-suffix"`
	ArtifactType       string `paramName:"artifact-type"`
	Source             string `paramName:"source"`
	ResultPathImageRef string `paramName:"result-path-image-ref"`
}

type PushContainerfileResults struct {
	ImageRef string `json:"image_ref"`
}

type PushContainerfile struct {
	Params        *PushContainerfileParams
	OrasClient    common.OrasClientInterface
	Results       PushContainerfileResults
	ResultsWriter common.ResultsWriterInterface

	imageName string
}

func NewPushContainerfile(cmd *cobra.Command) (*PushContainerfile, error) {
	params := &PushContainerfileParams{}
	if err := common.ParseParameters(cmd, PushContainerfileParamsConfig, params); err != nil {
		return nil, err
	}
	pushContainerfile := &PushContainerfile{
		Params:        params,
		OrasClient:    common.NewOrasClient(),
		ResultsWriter: common.NewResultsWriter(),
	}
	return pushContainerfile, nil
}

func (c *PushContainerfile) Run() error {
	c.logParams()

	imageUrl := c.Params.ImageUrl
	c.imageName = common.GetImageName(imageUrl)

	if err := c.validateParams(); err != nil {
		return err
	}

	curDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Error getting current directory: %w", err)
	}
	l.Logger.Debugf("Using current directory: %s\n", curDir)

	containerfilePath, err := common.SearchDockerfile(common.DockerfileSearchOpts{
		SourceDir:  c.Params.Source,
		ContextDir: c.Params.Context,
		Dockerfile: c.Params.Containerfile,
	})
	if err != nil {
		return fmt.Errorf("Error on searching Container: %w", err)
	}

	if containerfilePath == "" {
		l.Logger.Debugf("Containerfile '%s' is not found from source '%s' and context '%s'. Abort push.",
			c.Params.Containerfile, c.Params.Source, c.Params.Context)
		return nil
	}

	l.Logger.Debugf("Got Containerfile: %s", containerfilePath)

	l.Logger.Debugf("Select registry authentication for %s\n", imageUrl)
	registryAuth, err := common.SelectRegistryAuthFromDefaultAuthFile(imageUrl)
	if err != nil {
		return fmt.Errorf("Cannot select registry authentication for image %s: %w", imageUrl, err)
	}

	username, password, err := common.ExtractCredentials(registryAuth.Token)
	if err != nil {
		return fmt.Errorf("Error on extracting authentication credential: %w", err)
	}

	tag := c.generateContainerfileImageTag()

	absContainerfilePath, err := filepath.Abs(containerfilePath)
	if err != nil {
		return fmt.Errorf("Error on getting absolute path of %s: %w", containerfilePath, err)
	}
	remoteRepo := common.NewRepository(c.imageName, username, password)
	digest, err := c.OrasClient.Push(remoteRepo, tag, absContainerfilePath, c.Params.ArtifactType)
	if err != nil {
		return fmt.Errorf("Failed to push Containerfile %s: %w", containerfilePath, err)
	}

	l.Logger.Debugf("Containerfile '%s' is pushed to registry with tag: %s\n", containerfilePath, tag)

	artifactImageRef := fmt.Sprintf("%s@%s", c.imageName, digest)

	c.Results.ImageRef = artifactImageRef
	if resultsJson, err := c.ResultsWriter.CreateResultJson(c.Results); err != nil {
		return fmt.Errorf("Error on creating results JSON: %w", err)
	} else {
		fmt.Print(resultsJson)
	}

	if c.Params.ResultPathImageRef != "" {
		err = c.ResultsWriter.WriteResultString(artifactImageRef, c.Params.ResultPathImageRef)
		if err != nil {
			return fmt.Errorf("Error on writing result image digest: %w", err)
		}
	}

	return nil
}

func (c *PushContainerfile) generateContainerfileImageTag() string {
	digest := strings.Replace(c.Params.ImageDigest, ":", "-", 1)
	return digest + c.Params.TagSuffix
}

func (c *PushContainerfile) validateParams() error {
	if !common.IsImageNameValid(c.imageName) {
		return fmt.Errorf("image name '%s' is invalid", c.imageName)
	}

	if !common.IsImageDigestValid(c.Params.ImageDigest) {
		return fmt.Errorf("image digest '%s' is invalid", c.Params.ImageDigest)
	}

	tagSuffix := c.Params.TagSuffix
	if !regexp.MustCompile(tagSuffixRegex).MatchString(tagSuffix) {
		return fmt.Errorf("Tag suffix includes invalid characters or exceeds the max length of 57 characters.")
	}

	return nil
}

func (c *PushContainerfile) logParams() {
	l.Logger.Infof("[param] Image URL: %s", c.Params.ImageUrl)
	l.Logger.Infof("[param] Image digest: %s", c.Params.ImageDigest)
	l.Logger.Infof("[param] Tag suffix: %s", c.Params.TagSuffix)
	l.Logger.Infof("[param] Containerfile: %s", c.Params.Containerfile)
	l.Logger.Infof("[param] Context: %s", c.Params.Context)
	l.Logger.Infof("[param] Artifact type: %s", c.Params.ArtifactType)
	l.Logger.Infof("[param] Source directory: %s", c.Params.Source)
	l.Logger.Infof("[param] Image Reference result file: %s", c.Params.ResultPathImageRef)
}
