// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"github.com/juju/version/v2"
)

// ValidatorsForControllerUpgrade returns a list of validators for controller upgrade,
func ValidatorsForControllerUpgrade(isControllerModel bool, targetVersion version.Number) []Validator {
	if isControllerModel {
		validators := []Validator{
			getCheckTargetVersionForModel(targetVersion, UpgradeToAllowed),
			checkMongoStatusForControllerUpgrade,
		}
		if targetVersion.Major == 3 {
			validators = append(validators,
				checkMongoVersionForControllerModel,
				checkNoWinMachinesForModel,
				checkNoXenialMachinesForModel,
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
			checkNoWinMachinesForModel, checkNoXenialMachinesForModel,
		)
	}
	return validators
}

// ValidatorsForModelUpgrade returns a list of validators for model upgrade,
func ValidatorsForModelUpgrade(force bool, targetVersion version.Number) []Validator {
	validators := []Validator{
		getCheckUpgradeSeriesLockForModel(force),
	}
	if targetVersion.Major == 3 {
		validators = append(validators,
			checkNoWinMachinesForModel, checkNoXenialMachinesForModel,
		)
	}
	return validators
}
