// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg

import (
	"fmt"
	"strings"

	"github.com/juju/version"

	"github.com/juju/juju/controller"
)

const (
	jujudOCINamespace = "jujusolutions"
	jujudOCIName      = "jujud-operator"
	jujudbOCIName     = "juju-db"
)

// GetControllerImagePath returns oci image path of jujud for a controller.
func (cfg *ControllerPodConfig) GetControllerImagePath() string {
	return GetJujuOCIImagePath(cfg.Controller.Config, cfg.JujuVersion)
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

// IsJujuOCIImage returns true if the image path is for a Juju operator.
func IsJujuOCIImage(imagePath string) bool {
	return strings.Contains(imagePath, jujudOCIName+":")
}

// GetJujuOCIImagePath returns the jujud oci image path.
func GetJujuOCIImagePath(controllerCfg controller.Config, ver version.Number) string {
	// First check the deprecated "caas-operator-image-path" config.
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
		imageRepo = jujudOCINamespace
	}
	path := fmt.Sprintf("%s/%s", imageRepo, jujudOCIName)
	return tagImagePath(path, ver)
}
