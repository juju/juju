// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg

import (
	"fmt"
	"strings"

	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
)

const (
	JujudOCINamespace = "jujusolutions"
	JujudOCIName      = "jujud-operator"
	JujudbOCIName     = "juju-db"
	JujudbVersion     = "4.4"
)

// GetControllerImagePath returns oci image path of jujud for a controller.
func (cfg *ControllerPodConfig) GetControllerImagePath() string {
	return GetJujuOCIImagePath(cfg.Controller.Config, cfg.JujuVersion, cfg.OfficialBuild)
}

// GetJujuDbOCIImagePath returns the juju-db oci image path.
func (cfg *ControllerPodConfig) GetJujuDbOCIImagePath() string {
	imageRepo := cfg.Controller.Config.CAASImageRepo()
	if imageRepo == "" {
		imageRepo = JujudOCINamespace
	}
	return fmt.Sprintf("%s/%s:%v", imageRepo, JujudbOCIName, JujudbVersion)
}

// IsJujuOCIImage returns true if the image path is for a Juju operator.
func IsJujuOCIImage(imagePath string) bool {
	return strings.Contains(imagePath, JujudOCIName+":")
}

// GetJujuOCIImagePath returns the jujud oci image path.
func GetJujuOCIImagePath(controllerCfg controller.Config, ver version.Number, build int) string {
	// First check the deprecated "caas-operator-image-path" config.
	ver.Build = build
	imagePath := RebuildOldOperatorImagePath(
		controllerCfg.CAASOperatorImagePath(), ver,
	)
	if imagePath != "" {
		return imagePath
	}
	return imageRepoToPath(controllerCfg.CAASImageRepo(), ver)
}

// RebuildOldOperatorImagePath returns a updated image path for the specified juju version.
func RebuildOldOperatorImagePath(imagePath string, ver version.Number) string {
	if imagePath == "" {
		return ""
	}
	return tagImagePath(imagePath, ver)
}

func tagImagePath(path string, ver version.Number) string {
	var verString string
	splittedPath := strings.Split(path, ":")
	path = splittedPath[0]
	if len(splittedPath) > 1 {
		verString = splittedPath[1]
	}
	if ver != version.Zero {
		verString = ver.String()
	}
	if verString != "" {
		// tag with version.
		path += ":" + verString
	}
	return path
}

func imageRepoToPath(imageRepo string, ver version.Number) string {
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
