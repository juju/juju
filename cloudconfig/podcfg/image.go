// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg

import (
	"fmt"
	"strings"

	"github.com/docker/distribution/reference"
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
)

const (
	JujudOCINamespace = "jujusolutions"
	JujudOCIName      = "jujud-operator"
	JujudbOCIName     = "juju-db"
)

// GetControllerImagePath returns oci image path of jujud for a controller.
func (cfg *ControllerPodConfig) GetControllerImagePath() (string, error) {
	return GetJujuOCIImagePath(cfg.Controller.Config, cfg.JujuVersion, cfg.OfficialBuild)
}

// GetJujuDbOCIImagePath returns the juju-db oci image path.
func (cfg *ControllerPodConfig) GetJujuDbOCIImagePath() string {
	imageRepo := cfg.Controller.Config.CAASImageRepo()
	if imageRepo == "" {
		imageRepo = JujudOCINamespace
	}
	v := jujudbVersion
	return fmt.Sprintf("%s/%s:%d.%d", imageRepo, JujudbOCIName, v.Major, v.Minor)
}

// IsJujuOCIImage returns true if the image path is for a Juju operator.
func IsJujuOCIImage(imagePath string) bool {
	return strings.Contains(imagePath, JujudOCIName+":")
}

// GetJujuOCIImagePath returns the jujud oci image path.
func GetJujuOCIImagePath(controllerCfg controller.Config, ver version.Number, build int) (string, error) {
	// First check the deprecated "caas-operator-image-path" config.
	ver.Build = build
	imagePath, err := RebuildOldOperatorImagePath(
		controllerCfg.CAASOperatorImagePath(), ver,
	)
	if imagePath != "" || err != nil {
		return imagePath, err
	}
	return imageRepoToPath(controllerCfg.CAASImageRepo(), ver)
}

// RebuildOldOperatorImagePath returns a updated image path for the specified juju version.
func RebuildOldOperatorImagePath(imagePath string, ver version.Number) (string, error) {
	if imagePath == "" {
		return "", nil
	}
	return tagImagePath(imagePath, ver)
}

func tagImagePath(fullPath string, ver version.Number) (string, error) {
	ref, err := reference.Parse(fullPath)
	if err != nil {
		return "", errors.Trace(err)
	}
	imageNamed, ok := ref.(reference.Named)
	// Safety check only - should never happen.
	if !ok {
		return "", errors.Errorf("unexpected docker image path type, got %T, expected reference.Named", ref)
	}
	if ver != version.Zero {
		// ver is always a valid tag.
		imageNamed, _ = reference.WithTag(imageNamed, ver.String())
	}
	return imageNamed.String(), nil
}

func imageRepoToPath(imageRepo string, ver version.Number) (string, error) {
	if imageRepo == "" {
		imageRepo = JujudOCINamespace
	}
	path := fmt.Sprintf("%s/%s", imageRepo, JujudOCIName)
	return tagImagePath(path, ver)
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
	image := fmt.Sprintf("%s/charm-base:%s", imageRepo, tag)
	return image, nil
}
