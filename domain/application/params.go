// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

// AddUnitParams contains parameters for saving a unit to state.
type AddUnitParams struct {
	// UnitName is for CAAS models when creating stateful units.
	UnitName *string
}
