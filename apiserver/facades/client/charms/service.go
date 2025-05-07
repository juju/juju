// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ApplicationService provides access to application related operations, this
// includes charms, units and resources.
type ApplicationService interface {
	// SetCharm persists the charm metadata, actions, config and manifest to
	// state.
	// If there are any non-blocking issues with the charm metadata, actions,
	// config or manifest, a set of warnings will be returned.
	SetCharm(context.Context, charm.SetCharmArgs) (corecharm.ID, []string, error)

	// ListCharmLocators returns a list of charms with the specified
	// locator by the name. If no names are provided, all charms are returned.
	ListCharmLocators(ctx context.Context, names ...string) ([]charm.CharmLocator, error)

	// GetApplicationIDByName returns an application ID by application name. It
	// returns an error if the application can not be found by the name.
	//
	// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
	// and [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// IsSubordinateApplication returns true if the application is a subordinate
	// application.
	// The following errors may be returned:
	// - [appliationerrors.ApplicationNotFound] if the application does not exist
	IsSubordinateApplication(context.Context, coreapplication.ID) (bool, error)

	// GetApplicationConstraints returns the application constraints for the
	// specified application ID.
	// Empty constraints are returned if no constraints exist for the given
	// application ID.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConstraints(context.Context, coreapplication.ID) (constraints.Value, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineArchesForApplication returns a list of architectures which are
	// included across the machines of the given application.
	GetMachineArchesForApplication(context.Context, coreapplication.ID) ([]arch.Arch, error)
}
