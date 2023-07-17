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
	4: version.MustParse("3.1.0"),
}

// MinClientVersions defines the minimum user client version
// allowed to make a call to a controller with the major version,
// or the minimum controller version needed to accept a call from a
// client with the major version.
var MinClientVersions = map[int]version.Number{
	4: version.MustParse("3.1.0"),
}

// MinMajorMigrateVersions defines the minimum version the model
// must be running before migrating to the target controller.
var MinMajorMigrateVersions = MinAgentVersions

// MigrateToAllowed checks if the model can be migrated to the target controller.
func MigrateToAllowed(modelVersion, targetControllerVersion version.Number) (bool, version.Number, error) {
	return versionCheck(
		modelVersion, targetControllerVersion, MinMajorMigrateVersions, "migrate",
	)
}

// UpgradeControllerAllowed returns true if a controller upgrade is allowed
// when it hosts a model with the specified version.
func UpgradeControllerAllowed(modelVersion, targetControllerVersion version.Number) (bool, version.Number, error) {
	return versionCheck(
		modelVersion, targetControllerVersion, MinAgentVersions, "upgrading controller",
	)
}

func versionCheck(
	from, to version.Number, versionMap map[int]version.Number, operation string,
) (bool, version.Number, error) {
	// If the major version is the same then we will allow the upgrade.
	if from.Major == to.Major {
		return true, version.Number{}, nil
	}
	// Downgrades not allowed.
	if from.Major > to.Major {
		logger.Debugf("downgrade from %q to %q is not allowed", from, to)
		return false, version.Number{}, errors.Errorf("downgrade is not allowed")
	}

	minVer, ok := versionMap[to.Major]
	logger.Debugf("from %q, to %q, versionMap %#v", from, to, versionMap)
	if !ok {
		return false, version.Number{}, errors.Errorf("%s to %q is not supported from %q", operation, to, from)
	}

	// We don't want to allow upgrades from a version that is lower than the
	// minimum major version (for example, 2.9.0 to 4.0.0).
	if from.Major < minVer.Major {
		return false, version.Number{}, errors.Errorf("%s to %q is not supported from %q", operation, to, from)
	}

	// Allow upgrades from rc etc.
	from.Tag = ""
	return from.Compare(minVer) >= 0, minVer, nil
}
