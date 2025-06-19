// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg

import (
	"fmt"
	"strings"

	"github.com/distribution/reference"
	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/docker"
)

const (
	JujudOCINamespace = "docker.io/jujusolutions"
	JujudOCIName      = "jujud-operator"
	JujudbOCIName     = "juju-db"
	CharmBaseName     = "charm-base"
)

// GetControllerImagePath returns oci image path of jujud for a controller.
func (cfg *ControllerPodConfig) GetControllerImagePath() (string, error) {
	return GetJujuOCIImagePath(cfg.Controller, cfg.JujuVersion)
}

func (cfg *ControllerPodConfig) dbVersion() (version.Number, error) {
	snapChannel := cfg.Controller.JujuDBSnapChannel()
	vers := strings.Split(snapChannel, "/")[0] + ".0"
	return version.Parse(vers)
}

// GetJujuDbOCIImagePath returns the juju-db oci image path.
func (cfg *ControllerPodConfig) GetJujuDbOCIImagePath() (string, error) {
	details, err := docker.NewImageRepoDetails(cfg.Controller.CAASImageRepo())
	if err != nil {
		return "", errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	imageRepo := details.Repository
	if imageRepo == "" {
		imageRepo = JujudOCINamespace
	}
	path := fmt.Sprintf("%s/%s", imageRepo, JujudbOCIName)
	mongoVers, err := cfg.dbVersion()
	if err != nil {
		return "", errors.Annotatef(err, "cannot parse %q from controller config", controller.JujuDBSnapChannel)
	}
	tag := fmt.Sprintf("%d.%d", mongoVers.Major, mongoVers.Minor)
	return tagImagePath(path, tag)
}

// IsJujuOCIImage returns true if the image path is for a Juju operator.
func IsJujuOCIImage(imagePath string) bool {
	return strings.Contains(imagePath, JujudOCIName+":")
}

// IsCharmBaseImage returns true if the image path is for a Juju operator.
func IsCharmBaseImage(imagePath string) bool {
	return strings.Contains(imagePath, CharmBaseName+":")
}

// GetJujuOCIImagePath returns the jujud oci image path.
func GetJujuOCIImagePath(controllerCfg controller.Config, ver version.Number) (string, error) {
	// First check the deprecated "caas-operator-image-path" config.
	imagePath, err := RebuildOldOperatorImagePath(
		controllerCfg.CAASOperatorImagePath(), ver,
	)
	if imagePath != "" || err != nil {
		return imagePath, err
	}
	details, err := docker.NewImageRepoDetails(controllerCfg.CAASImageRepo())
	if err != nil {
		return "", errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	tag := ""
	if ver != version.Zero {
		tag = ver.String()
	}
	return imageRepoToPath(details.Repository, tag)
}

// RebuildOldOperatorImagePath returns a updated image path for the specified juju version.
func RebuildOldOperatorImagePath(imagePath string, ver version.Number) (string, error) {
	if imagePath == "" {
		return "", nil
	}
	tag := ""
	if ver != version.Zero {
		// ver is always a valid tag.
		tag = ver.String()
	}
	return tagImagePath(imagePath, tag)
}

func tagImagePath(fullPath, tag string) (string, error) {
	ref, err := reference.Parse(fullPath)
	if err != nil {
		return "", errors.Trace(err)
	}
	imageNamed, ok := ref.(reference.Named)
	// Safety check only - should never happen.
	if !ok {
		return "", errors.Errorf("unexpected docker image path type, got %T, expected reference.Named", ref)
	}
	if tag != "" {
		imageNamed, _ = reference.WithTag(imageNamed, tag)
	}
	return imageNamed.String(), nil
}

func imageRepoToPath(imageRepo, tag string) (string, error) {
	if imageRepo == "" {
		imageRepo = JujudOCINamespace
	}
	path := fmt.Sprintf("%s/%s", imageRepo, JujudOCIName)
	return tagImagePath(path, tag)
}

func RecoverRepoFromOperatorPath(fullpath string) (string, error) {
	split := strings.Split(fullpath, JujudOCIName)
	if len(split) != 2 {
		return "", errors.Errorf("image path %q does not match the form somerepo/%s:.*", fullpath, JujudOCIName)
	}
	return strings.TrimRight(split[0], "/"), nil
}

// ImageForBase returns the OCI image path for a generic base.
// NOTE: resource referenced bases are not resolved via ImageForBase.
func ImageForBase(imageRepo string, base charm.Base) (string, error) {
	if base.Name == "" {
		return "", errors.NotValidf("empty base name")
	}
	if imageRepo == "" {
		imageRepo = JujudOCINamespace
	}
	if len(base.Channel.Track) == 0 || len(base.Channel.Risk) == 0 {
		return "", errors.NotValidf("channel %q", base.Channel)
	}
	tag := fmt.Sprintf("%s-%s", base.Name, base.Channel.Track)
	if base.Channel.Risk != charm.Stable {
		tag = fmt.Sprintf("%s-%s", tag, base.Channel.Risk)
	}
	image := fmt.Sprintf("%s/%s:%s", imageRepo, CharmBaseName, tag)
	return image, nil
}
