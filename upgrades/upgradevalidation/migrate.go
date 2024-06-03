// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// ValidatorsForModelMigrationSource returns a list of validators to run source
// controller for model migration.
// Note: the target version can never be lower than the current version.
func ValidatorsForModelMigrationSource(
	cloudspec environscloudspec.CloudSpec,
) []Validator {
	return []Validator{
		getCheckUpgradeSeriesLockForModel(false),
		checkNoWinMachinesForModel,
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
		checkForCharmStoreCharms,
		checkFanNetworksAndContainers,
	}
}
