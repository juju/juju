// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/life"
)

// StateTxOperationFunc is a closure which is passed to state calls to allow the service caller
// to run business logic in the context of a single transaction.
type StateTxOperationFunc func(ctx context.Context, stateOps StateTxOperations, appID coreapplication.ID) error

// StateTxOperations define state layer operations used by business logic in the
// service layer, all of which need to run in a single transaction.
type StateTxOperations interface {
	// UpsertUnit creates or updates the specified application unit, returning an error
	// satisfying [applicationerrors.ApplicationNotFoundError] if the application doesn't exist.
	UpsertUnit(context.Context, coreapplication.ID, UpsertUnitArg) error

	// ApplicationScaleState looks up the scale state of the specified application, returning an error
	// satisfying [applicationerrors.ApplicationNotFound] if the application is not found.
	ApplicationScaleState(context.Context, coreapplication.ID) (ScaleState, error)

	// UnitLife looks up the life of the specified unit, returning an error
	// satisfying [uniterrors.NotFound] if the unit is not found.
	UnitLife(ctx context.Context, unitName string) (life.Life, error)
}

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

// UpsertUnitArg contains parameters for adding a unit to state.
type UpsertUnitArg struct {
	UnitName       *string
	PasswordHash   *string
	CloudContainer *CloudContainer
}
