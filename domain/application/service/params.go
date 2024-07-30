// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// AddApplicationArgs contain arguments for adding an application to the model.
type AddApplicationArgs struct {
	// Storage contains the application's storage directives.
	Storage map[string]storage.Directive
}

// AddUnitArgs contains parameters for adding a unit to the model.
type AddUnitArg struct {
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

// UpdateCharmParams contains the parameters for updating
// an application's charm and storage.
type UpdateCharmParams struct {
	// Charm is the new charm to use for the application. New units
	// will be started with this charm, and existing units will be
	// upgraded to use it.
	Charm charm.Charm

	// Storage contains the storage directives to add or update when
	// upgrading the charm.
	//
	// Any existing storage instances for the named stores will be
	// unaffected; the storage directives will only be used for
	// provisioning new storage instances.
	Storage map[string]storage.Directive
}
