// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ModelService provides a subset of the model domain service methods.
type ModelService interface {
	// GetModelConstraints returns the current model's constraints.
	GetModelConstraints(context.Context) (constraints.Value, error)
}

// ApplicationService provides access to application related operations, this
// includes charms, units and resources.
type ApplicationService interface {
	// SetCharm persists the charm metadata, actions, config and manifest to
	// state.
	// If there are any non-blocking issues with the charm metadata, actions,
	// config or manifest, a set of warnings will be returned.
	SetCharm(ctx context.Context, args charm.SetCharmArgs) (corecharm.ID, []string, error)

	// ListCharmLocators returns a list of charms with the specified
	// locator by the name. If no names are provided, all charms are returned.
	ListCharmLocators(ctx context.Context, names ...string) ([]charm.CharmLocator, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (string, error)
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID string) (*instance.HardwareCharacteristics, error)
}
