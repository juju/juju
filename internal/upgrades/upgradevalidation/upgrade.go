// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/juju/core/semversion"
)

// ValidatorsForControllerModelUpgrade returns a list of validators for the
// controller model in a controller upgrade.
// Note: the target version can never be lower than the current version.
func ValidatorsForControllerModelUpgrade(
	targetVersion semversion.Number,
) []Validator {
	validators := []Validator{
		getCheckTargetVersionForControllerModel(targetVersion),
		checkForDeprecatedUbuntuSeriesForModel,
	}
	return validators
}

// ModelValidatorsForControllerModelUpgrade returns a list of validators for
// non-controller models in a controller upgrade.
// Note: the target version can never be lower than the current version.
func ModelValidatorsForControllerModelUpgrade(
	targetVersion semversion.Number,
) []Validator {
	validators := []Validator{
<<<<<<< HEAD:internal/upgrades/upgradevalidation/upgrade.go
		getCheckTargetVersionForModel(targetVersion, UpgradeControllerAllowed),
=======
		getCheckTargetVersionForModel(targetVersion),
		checkModelMigrationModeForControllerUpgrade,
		checkNoWinMachinesForModel,
>>>>>>> 3.6:upgrades/upgradevalidation/upgrade.go
		checkForDeprecatedUbuntuSeriesForModel,
	}
	return validators
}

// ValidatorsForModelUpgrade returns a list of validators for model upgrade.
// Note: the target version can never be lower than the current version.
func ValidatorsForModelUpgrade(
	force bool, targetVersion semversion.Number,
) []Validator {
	validators := []Validator{
		checkForDeprecatedUbuntuSeriesForModel,
	}
	return validators
}
