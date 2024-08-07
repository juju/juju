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
}

// Platform contains parameters for an application's platform.
type Platform struct {
	Channel        string
	OSTypeID       OSType
	ArchitectureID Architecture
}

type CloudContainer struct {
	ProviderId *string
	Address    *Address
	Ports      *[]string
}

type Address struct {
	Value       string
	AddressType string
	Scope       string
	Origin      string
	SpaceID     string
}

// AddUnitArg contains parameters for saving a unit to state.
type AddUnitArg struct {
	UnitName       *string
	PasswordHash   *string
	CloudContainer *CloudContainer
}
