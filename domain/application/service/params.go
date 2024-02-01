// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

// AddUnitParams contains parameters for adding a unit to the model.
type AddUnitParams struct {
	// UnitName is for CAAS models when creating stateful units.
	UnitName *string
}
