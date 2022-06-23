// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"
	"github.com/juju/version/v2"
)

// MinMajorMigrateVersion defines the minimum version the model
// must be running before migrating to the target controller.
var MinMajorMigrateVersion = map[int]version.Number{
	3: version.MustParse("2.8.9"),
}

// MigrateAllowed checks if the model can be migrated to the target controller.
func MigrateAllowed(modelVersion, targetControllerVersion version.Number) (bool, version.Number, error) {
	return versionCheck(modelVersion, targetControllerVersion, MinMajorMigrateVersion)
}

// MinMajorUpgradeVersion defines the minimum version all models
// must be running before a major version upgrade.
var MinMajorUpgradeVersion = map[int]version.Number{
	// TODO: enable here once we fix the capped txn collection issue in juju3.
	// 3: version.MustParse("2.9.33"),
}

// UpgradeAllowed returns true if a major version upgrade is allowed
// for the specified from and to versions.
func UpgradeAllowed(from, to version.Number) (bool, version.Number, error) {
	return versionCheck(from, to, MinMajorUpgradeVersion)
}

func versionCheck(from, to version.Number, versionMap map[int]version.Number) (bool, version.Number, error) {
	if from.Major == to.Major {
		return true, version.Number{}, nil
	}
	// Downgrades not allowed.
	if from.Major > to.Major {
		return false, version.Number{}, nil
	}
	minVer, ok := versionMap[to.Major]
	if !ok {
		return false, version.Number{}, errors.Errorf("unknown version %q", to)
	}
	// Allow upgrades from rc etc.
	from.Tag = ""
	return from.Compare(minVer) >= 0, minVer, nil
}

// TODO: add tests
