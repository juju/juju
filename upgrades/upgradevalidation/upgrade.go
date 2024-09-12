// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/version/v2"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// ValidatorsForControllerModelUpgrade returns a list of validators for the
// controller model in a controller upgrade.
// Note: the target version can never be lower than the current version.
func ValidatorsForControllerModelUpgrade(
	targetVersion version.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	validators := []Validator{
		getCheckTargetVersionForControllerModel(targetVersion),
		checkMongoStatusForControllerUpgrade,
		checkMongoVersionForControllerModel,
		checkNoWinMachinesForModel,
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
	}
	if targetVersion.Major == 3 && targetVersion.Minor >= 1 {
		validators = append(validators, checkForCharmStoreCharms)
	}
	return validators
}

// ModelValidatorsForControllerModelUpgrade returns a list of validators for
// non-controller models in a controller upgrade.
// Note: the target version can never be lower than the current version.
func ModelValidatorsForControllerModelUpgrade(
	targetVersion version.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	validators := []Validator{
		getCheckTargetVersionForModel(targetVersion, UpgradeControllerAllowed),
		checkModelMigrationModeForControllerUpgrade,
		checkNoWinMachinesForModel,
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
	}
	if targetVersion.Major == 3 && targetVersion.Minor >= 1 {
		validators = append(validators, checkForCharmStoreCharms)
	}
	return validators
}

// ValidatorsForModelUpgrade returns a list of validators for model upgrade.
// Note: the target version can never be lower than the current version.
func ValidatorsForModelUpgrade(
	force bool, targetVersion version.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	validators := []Validator{
		getCheckUpgradeSeriesLockForModel(force),
		checkNoWinMachinesForModel,
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
	}
	if targetVersion.Major == 3 && targetVersion.Minor >= 1 {
		validators = append(validators, checkForCharmStoreCharms)
	}
	return validators
}
