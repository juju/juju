// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/version/v2"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// ValidatorsForModelMigrationSource returns a list of validators to run source controller for model migration,
func ValidatorsForModelMigrationSource(
	targetVersion version.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	validators := []Validator{
		getCheckTargetVersionForModel(targetVersion, MigrateToAllowed),
		getCheckUpgradeSeriesLockForModel(false),
	}
	if targetVersion.Major == 3 {
		validators = append(validators,
			checkNoWinMachinesForModel,
			checkForDeprecatedUbuntuSeriesForModel,
			getCheckForLXDVersion(cloudspec),
		)
		if targetVersion.Minor >= 1 {
			validators = append(validators, checkForCharmStoreCharms)
		}
	}
	return validators
}
