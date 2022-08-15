// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/version/v2"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// ValidatorsForControllerUpgrade returns a list of validators for controller upgrade,
func ValidatorsForControllerUpgrade(
	isControllerModel bool, targetVersion version.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	if isControllerModel {
		validators := []Validator{
			getCheckTargetVersionForModel(targetVersion, UpgradeToAllowed),
			checkMongoStatusForControllerUpgrade,
		}
		if targetVersion.Major == 3 {
			validators = append(validators,
				checkMongoVersionForControllerModel,
				checkNoWinMachinesForModel,
				checkForDeprecatedUbuntuSeriesForModel,
				getCheckForLXDVersion(cloudspec, minLXDVersion),
			)
		}
		return validators
	}
	validators := []Validator{
		getCheckTargetVersionForModel(targetVersion, UpgradeToAllowed),
		checkModelMigrationModeForControllerUpgrade,
	}
	if targetVersion.Major == 3 {
		validators = append(validators,
			checkNoWinMachinesForModel,
			checkForDeprecatedUbuntuSeriesForModel,
			getCheckForLXDVersion(cloudspec, minLXDVersion),
		)
	}
	return validators
}

// ValidatorsForModelUpgrade returns a list of validators for model upgrade,
func ValidatorsForModelUpgrade(
	force bool, targetVersion version.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	validators := []Validator{
		getCheckUpgradeSeriesLockForModel(force),
	}
	if targetVersion.Major == 3 {
		validators = append(validators,
			checkNoWinMachinesForModel,
			checkForDeprecatedUbuntuSeriesForModel,
			getCheckForLXDVersion(cloudspec, minLXDVersion),
		)
	}
	return validators
}
