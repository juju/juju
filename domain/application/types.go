// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	domaincharm "github.com/juju/juju/domain/application/charm"
)

// AddApplicationArg contains parameters for saving an application to state.
type AddApplicationArg struct {
	Charm    domaincharm.Charm
	Channel  *domaincharm.Channel
	Platform Platform
	Origin   Origin
}

// Platform contains parameters for an application's platform.
type Platform struct {
	Channel        string
	OSTypeID       OSType
	ArchitectureID Architecture
}

// ScaleState describes the scale status of a k8s application.
type ScaleState struct {
	Scaling     bool
	Scale       int
	ScaleTarget int
}

// CloudService contains parameters for an application's cloud service.
type CloudService struct {
	ProviderId string
	Address    *Address
}

// Origin contains parameters for an application's origin.
type Origin struct {
	Revision int
}

// CloudContainer contains parameters for a unit's cloud container.
type CloudContainer struct {
	ProviderId *string
	Address    *Address
	Ports      *[]string
}

// Address contains parameters for a cloud container address.
type Address struct {
	Value       string
	AddressType string
	Scope       string
	Origin      string
	SpaceID     string
}

// UpsertUnitArg contains parameters for adding a unit to state.
type UpsertUnitArg struct {
	UnitName       *string
	PasswordHash   *string
	CloudContainer *CloudContainer
}
