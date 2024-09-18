// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// ApplicationServiceParams defines parameters used to
// create an application service.
type ApplicationServiceParams struct {
	StorageRegistry storage.ProviderRegistry
	Secrets         SecretService
}

// AddApplicationArgs contains arguments for adding an application to the model.
type AddApplicationArgs struct {
	// ReferenceName is the given name of the charm that is stored in the
	// persistent storage. The proxy name should either be the application
	// name or the charm metadata name.
	//
	// The name of a charm can differ from the charm name stored in the metadata
	// in the cases where the application name is selected by the user.
	// In order to select that charm again via the name, we need to use the
	// proxy name to locate it. You can't go via the application and select it
	// via the application name, as no application might be referencing it at
	// that specific revision. The only way to then locate the charm directly
	// via the name is use the proxy name.
	ReferenceName string
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
	UnitName       string
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
