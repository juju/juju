// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/distribution/reference"
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/controller"
	jujuversion "github.com/juju/juju/version"
)

const (
	jujudOCINamespace = "jujusolutions"
	jujudOCIName      = "jujud-operator"
	jujudOCINameGit   = "jujud-operator-git"
	jujudbOCIName     = "juju-db"
)

var (
	matchJujudOCIName    = matchImageName(jujudOCIName)
	matchJujudOCINameGit = matchImageName(jujudOCINameGit)
	matchJujudbOCIName   = matchImageName(jujudbOCIName)
)

// GetControllerImagePath returns oci image path of jujud for a controller.
func (cfg *ControllerPodConfig) GetControllerImagePath() ([]string, error) {
	return GetJujuOCIImagePaths(cfg.Controller.Config, cfg.JujuVersion)
}

// GetJujuDbOCIImagePath returns the juju-db oci image path.
func (cfg *ControllerPodConfig) GetJujuDbOCIImagePath() string {
	imageRepo := cfg.Controller.Config.CAASImageRepo()
	if imageRepo == "" {
		imageRepo = jujudOCINamespace
	}
	v := jujudbVersion
	return fmt.Sprintf("%s/%s:%d.%d", imageRepo, jujudbOCIName, v.Major, v.Minor)
}

// IsJujuOCIImage returns true if the image path is for a Juju operator image.
func IsJujuOCIImage(imagePath string) bool {
	ref, err := reference.Parse(imagePath)
	if err != nil {
		return false
	}
	if namedRef, ok := ref.(reference.Named); ok {
		return matchJujudOCIName.MatchString(namedRef.Name()) ||
			matchJujudOCINameGit.MatchString(namedRef.Name())
	}
	return false
}

// IsJujuDBOCIImage returns true if the image path is for a Juju DB image.
func IsJujuDBOCIImage(imagePath string) bool {
	ref, err := reference.Parse(imagePath)
	if err != nil {
		return false
	}
	if namedRef, ok := ref.(reference.Named); ok {
		return matchJujudbOCIName.MatchString(namedRef.Name())
	}
	return false
}

// GetJujuOCIImagePath returns the jujud oci image path.
func GetJujuOCIImagePath(controllerCfg controller.Config, ver version.Number) (string, error) {
	imagePaths, err := GetJujuOCIImagePaths(controllerCfg, ver)
	if err != nil {
		return "", errors.Trace(err)
	}
	// The last image path should always be the versioned path.
	return imagePaths[len(imagePaths)-1], nil
}

// NormaliseImagePath removes tags or digest reference from the image path.
// jujusolutions/jujud-operator:2.6.5 => jujusolutions/jujud-operator
// jujusolutions/jujud-operator@sha256:0f...f => jujusolutions/jujud-operator
func NormaliseImagePath(imagePath string) (string, error) {
	ref, err := reference.Parse(imagePath)
	if err != nil {
		return "", errors.Annotatef(err, "failed to parse image ref %s", imagePath)
	}
	namedRef, ok := ref.(reference.Named)
	if !ok {
		return "", errors.Errorf("image ref %s does not name a repo", imagePath)
	}
	return reference.TrimNamed(namedRef).String(), nil
}

// GetJujuOCIImagePaths returns the jujud oci image paths. This can return more than one image
// path, sorted based on priority.
func GetJujuOCIImagePaths(controllerCfg controller.Config, ver version.Number) ([]string, error) {
	// First check the deprecated "caas-operator-image-path" config.
	imagePaths, err := RebuildOldOperatorImagePath(
		controllerCfg.CAASOperatorImagePath(), ver,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if imagePaths != nil {
		return imagePaths, nil
	}
	repo := controllerCfg.CAASImageRepo()
	if repo == "" {
		repo = jujudOCINamespace
	}
	return tagOperatorImagePaths(repo, ver)
}

// RebuildOldOperatorImagePath returns a updated image path for the specified juju version.
func RebuildOldOperatorImagePath(imagePath string, targetVer version.Number) ([]string, error) {
	if imagePath == "" {
		return nil, nil
	}
	ref, err := reference.Parse(imagePath)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to parse image ref %s", imagePath)
	}
	namedRef, ok := ref.(reference.Named)
	if !ok {
		return nil, errors.Errorf("image ref %s does not name a repo", imagePath)
	}
	repo := namedRef.Name()
	if matchJujudOCIName.MatchString(repo) {
		repo = strings.TrimSuffix(repo, jujudOCIName)
	} else if matchJujudOCINameGit.MatchString(repo) {
		repo = strings.TrimSuffix(repo, jujudOCINameGit)
	} else {
		// We don't understand this repo, just update the version tag.
		targetVer.Build = 0
		taggedRef, err := reference.WithTag(reference.TrimNamed(namedRef), targetVer.String())
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []string{taggedRef.String()}, nil
	}
	return tagOperatorImagePaths(repo, targetVer)
}

func tagOperatorImagePaths(repo string, targetVer version.Number) ([]string, error) {
	if repo != "" && !strings.HasSuffix(repo, "/") {
		repo += "/"
	}
	var res []string
	currentVer := jujuversion.Current
	currentVer.Build = 0
	targetVer.Build = 0
	if currentVer.Compare(targetVer) == 0 && jujuversion.GitCommit != "" {
		// If the current binary matches the requested version,
		// the preffered container image should be the exact version.
		tagHash := jujuversion.GitCommit
		namedRef, err := reference.WithName(repo + jujudOCINameGit)
		if err != nil {
			return nil, errors.Trace(err)
		}
		taggedRef, err := reference.WithTag(namedRef, fmt.Sprintf("%s-%s", targetVer.String(), tagHash))
		if err != nil {
			return nil, errors.Trace(err)
		}
		res = append(res, taggedRef.String())
	}
	namedRef, err := reference.WithName(repo + jujudOCIName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	taggedRef, err := reference.WithTag(namedRef, targetVer.String())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return append(res, taggedRef.String()), nil
}

func matchImageName(imageName string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(?:/|^)(?:%s)$`, imageName))
}
