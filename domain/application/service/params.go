// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

// AddApplicationParams contain parameters for adding an application to the model.
type AddApplicationParams struct {
	// Application constraints go here.
	// Storage constraints go here.
}

// AddUnitParams contains parameters for adding a unit to the model.
type AddUnitParams struct {
	// UnitName is for migration, adding named units.
	UnitName *string

	// Storage params go here.
}

// UpsertCAASUnitParams contain parameters for introducing
// a k8s unit representing a new pod to the model.
type UpsertCAASUnitParams struct {
	// UnitName is for CAAS models when creating stateful units.
	UnitName *string
}
