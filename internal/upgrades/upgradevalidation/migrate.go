// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

// ValidatorsForModelMigrationSource returns a list of validators to run source
// controller for model migration.
// Note: the target version can never be lower than the current version.
func ValidatorsForModelMigrationSource() []Validator {
	return []Validator{
		checkForDeprecatedUbuntuSeriesForModel,
	}
}
