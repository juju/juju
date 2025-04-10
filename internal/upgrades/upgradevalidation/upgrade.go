// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/juju/core/semversion"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// ValidatorsForControllerModelUpgrade returns a list of validators for the
// controller model in a controller upgrade.
// Note: the target version can never be lower than the current version.
func ValidatorsForControllerModelUpgrade(
	targetVersion semversion.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	validators := []Validator{
		getCheckTargetVersionForControllerModel(targetVersion),
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
	}
	return validators
}

// ModelValidatorsForControllerModelUpgrade returns a list of validators for
// non-controller models in a controller upgrade.
// Note: the target version can never be lower than the current version.
func ModelValidatorsForControllerModelUpgrade(
	targetVersion semversion.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	validators := []Validator{
		getCheckTargetVersionForModel(targetVersion, UpgradeControllerAllowed),
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
	}
	return validators
}

// ValidatorsForModelUpgrade returns a list of validators for model upgrade.
// Note: the target version can never be lower than the current version.
func ValidatorsForModelUpgrade(
	force bool, targetVersion semversion.Number, cloudspec environscloudspec.CloudSpec,
) []Validator {
	validators := []Validator{
		checkForDeprecatedUbuntuSeriesForModel,
		getCheckForLXDVersion(cloudspec),
	}
	return validators
}
