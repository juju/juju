// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// AddApplicationArgs contains arguments for adding an application to the model.
type AddApplicationArgs struct {
	// Storage contains the application's storage directives.
	Storage map[string]storage.Directive
}

// CloudContainerParams contains parameters for a unit cloud container.
type CloudContainerParams struct {
	ProviderId    *string
	Address       *network.SpaceAddress
	AddressOrigin *network.Origin
	Ports         *[]string
}

// AddressParams contains parameters for a unit/cloud container address.
type AddressParams struct {
	Value       string
	AddressType string
	Scope       string
	Origin      string
	SpaceID     string
}

// AddUnitArg contains parameters for adding a unit to the model.
type AddUnitArg struct {
	UnitName       *string
	PasswordHash   *string
	CloudContainer *CloudContainerParams

	// Storage params go here.
}

// ScalingState contains attributes that describes
// the scaling state of a CAAS application.
type ScalingState struct {
	ScaleTarget int
	Scaling     bool
}

// RegisterCAASUnitParams contains parameters for introducing
// a k8s unit representing a new pod to the model.
type RegisterCAASUnitParams struct {
	UnitName     string
	PasswordHash *string
	ProviderId   *string
	Address      *string
	Ports        *[]string
	OrderedScale bool
	OrderedId    int
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
