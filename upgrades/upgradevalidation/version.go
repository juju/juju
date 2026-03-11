// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version/v2"
)

var logger = loggo.GetLogger("juju.upgrades.validations")

// MinAgentVersions defines the minimum agent version
// allowed to make a call to a controller with the major version.
var MinAgentVersions = map[int]version.Number{
	3: version.MustParse("2.9.43"),
}

// MinMajorMigrateVersions defines the minimum version the model
// must be running before migrating to the target controller.
var MinMajorMigrateVersions = MinAgentVersions

// MigrateToAllowed checks if the model can be migrated to the target controller.
func MigrateToAllowed(modelVersion, targetControllerVersion version.Number) (bool, version.Number, error) {
	// If the major version is the same then we will allow the upgrade.
	if modelVersion.Major == targetControllerVersion.Major {
		return true, version.Number{}, nil
	}
	// Downgrades not allowed.
	if modelVersion.Major > targetControllerVersion.Major {
		logger.Debugf("downgrade from %q to %q is not allowed", modelVersion, targetControllerVersion)
		return false, version.Number{}, errors.Errorf("downgrade is not allowed")
	}

	minVer, ok := MinMajorMigrateVersions[targetControllerVersion.Major]
	logger.Debugf("from %q, to %q, versionMap %#v", modelVersion, targetControllerVersion, MinMajorMigrateVersions)
	if !ok {
		return false, version.Number{}, errors.Errorf("%s to %q is not supported from %q", "migrate", targetControllerVersion, modelVersion)
	}
	// Allow upgrades from rc etc.
	modelVersion.Tag = ""
	return modelVersion.Compare(minVer) >= 0, minVer, nil
}
