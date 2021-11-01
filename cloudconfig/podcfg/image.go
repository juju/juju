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
	"github.com/juju/juju/mongo"
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

func (cfg *ControllerPodConfig) mongoVersion() (*mongo.Version, error) {
	snapChannel := cfg.Controller.Config.JujuDBSnapChannel()
	vers := strings.Split(snapChannel, "/")[0] + ".0"
	versionNum, err := version.Parse(vers)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid mongo version %q in %q controller config", versionNum, controller.JujuDBSnapChannel)
	}
	mongoVersion := mongo.Mongo4xwt
	mongoVersion.Major = versionNum.Major
	mongoVersion.Minor = versionNum.Minor
	return &mongoVersion, nil
}

// GetJujuDbOCIImagePath returns the juju-db oci image path.
func (cfg *ControllerPodConfig) GetJujuDbOCIImagePath() (string, error) {
	imageRepo := cfg.Controller.Config.CAASImageRepo().Repository
	if imageRepo == "" {
		imageRepo = JujudOCINamespace
	}
	path := fmt.Sprintf("%s/%s", imageRepo, JujudbOCIName)
	mongoVers, err := cfg.mongoVersion()
	if err != nil {
		return "", errors.Trace(err)
	}
	tag := fmt.Sprintf("%d.%d", mongoVers.Major, mongoVers.Minor)
	return tagImagePath(path, tag)
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
		controllerCfg.CAASOperatorImagePath().Repository, ver,
	)
	if imagePath != "" || err != nil {
		return imagePath, err
	}
	tag := ""
	if ver != version.Zero {
		tag = ver.String()
	}
	return imageRepoToPath(controllerCfg.CAASImageRepo().Repository, tag)
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
