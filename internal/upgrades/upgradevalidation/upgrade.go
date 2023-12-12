// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/version/v2"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// ValidatorsForControllerUpgrade returns a list of validators for controller
// upgrade.
// Note: the target version can never be lower than the current version.
func ValidatorsForControllerUpgrade(
	isControllerModel bool, targetVersion version.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	if isControllerModel {
		validators := []Validator{
			getCheckTargetVersionForControllerModel(targetVersion),
			checkMongoStatusForControllerUpgrade,
			checkMongoVersionForControllerModel,
			checkForDeprecatedUbuntuSeriesForModel,
			getCheckForLXDVersion(cloudspec),
		}
		return validators
	}

	validators := []Validator{
		getCheckTargetVersionForModel(targetVersion, UpgradeControllerAllowed),
		checkModelMigrationModeForControllerUpgrade,
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
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
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
	}
	return validators
}
