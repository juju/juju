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
	3: version.MustParse("2.9.36"),
}

// MinClientVersions defines the minimum user client version
// allowed to make a call to a controller with the major version,
// or the minimum controller version needed to accept a call from a
// client with the major version.
var MinClientVersions = map[int]version.Number{
	3: version.MustParse("2.9.42"),
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

// MinMajorUpgradeVersions defines the minimum version all models
// must be running before a major version upgrade.
// var MinMajorUpgradeVersions = MinAgentVersions // We don't support upgrading in place from 2.9 to 3.0 yet.
var MinMajorUpgradeVersions = map[int]version.Number{}

// UpgradeToAllowed returns true if a major version upgrade is allowed
// for the specified from and to versions.
func UpgradeToAllowed(from, to version.Number) (bool, version.Number, error) {
	return versionCheck(
		from, to, MinMajorUpgradeVersions, "upgrade",
	)
}

func versionCheck(
	from, to version.Number, versionMap map[int]version.Number, operation string,
) (bool, version.Number, error) {
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
	// Allow upgrades from rc etc.
	from.Tag = ""
	return from.Compare(minVer) >= 0, minVer, nil
}
