// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
)

// StateOperationFunc is a closure which is passed to state calls to allow the service caller
// to run business logic in the context of a single transaction.
type StateOperationFunc func(ctx context.Context, tx *sqlair.TX, appID coreapplication.ID) error

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

// ScaleState describes the scale status of a k8s application.
type ScaleState struct {
	Scaling     bool
	Scale       int
	ScaleTarget int
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

// AddUnitArg contains parameters for saving a unit to state.
type AddUnitArg struct {
	UnitName       *string
	PasswordHash   *string
	CloudContainer *CloudContainer
}
