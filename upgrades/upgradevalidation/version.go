// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version/v2"
)

var logger = loggo.GetLogger("juju.upgrades.validations")

// MinMajorMigrateVersion defines the minimum version the model
// must be running before migrating to the target controller.
var MinMajorMigrateVersion = map[int]version.Number{
	3: version.MustParse("2.9.32"),
}

// MigrateToAllowed checks if the model can be migrated to the target controller.
func MigrateToAllowed(modelVersion, targetControllerVersion version.Number) (bool, version.Number, error) {
	return versionCheck(modelVersion, targetControllerVersion, MinMajorMigrateVersion)
}

// MinMajorUpgradeVersion defines the minimum version all models
// must be running before a major version upgrade.
var MinMajorUpgradeVersion = map[int]version.Number{
	// TODO: enable here once we fix the capped txn collection issue in juju3.
	// 3: version.MustParse("2.9.33"),
}

// UpgradeToAllowed returns true if a major version upgrade is allowed
// for the specified from and to versions.
func UpgradeToAllowed(from, to version.Number) (bool, version.Number, error) {
	return versionCheck(from, to, MinMajorUpgradeVersion)
}

func versionCheck(from, to version.Number, versionMap map[int]version.Number) (bool, version.Number, error) {
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
		return false, version.Number{}, errors.Errorf("%q is not a supported version", to)
	}
	// Allow upgrades from rc etc.
	from.Tag = ""
	return from.Compare(minVer) >= 0, minVer, nil
}
