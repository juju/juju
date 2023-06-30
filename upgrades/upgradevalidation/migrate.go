// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/version/v2"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// ValidatorsForModelMigrationSource returns a list of validators to run source
// controller for model migration.
// Note: the target version can never be lower than the current version.
func ValidatorsForModelMigrationSource(
	targetVersion version.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	validators := []Validator{
		getCheckTargetVersionForModel(targetVersion, MigrateToAllowed),
		getCheckUpgradeSeriesLockForModel(false),
		checkNoWinMachinesForModel,
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
	}
	// If the target version is 3.1 or greater, we need to check for charm
	// store charms and prevent them from being migrated.
	if targetVersion.Major > 3 || (targetVersion.Major == 3 && targetVersion.Minor >= 1) {
		validators = append(validators, checkForCharmStoreCharms)
	}
	return validators
}
