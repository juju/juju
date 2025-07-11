// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"context"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetMachineAddresses retrieves the network space addresses of a machine
	// identified by its UUID.
	GetMachineAddresses(ctx context.Context, uuid machine.UUID) (network.SpaceAddresses, error)
	// GetMachinePublicAddress retrieves the public address of a machine identified
	// by its UUID.
	GetMachinePublicAddress(ctx context.Context, uuid machine.UUID) (network.SpaceAddress, error)
	// GetMachinePrivateAddress retrieves the private address of a machine identified
	// by its UUID.
	GetMachinePrivateAddress(ctx context.Context, uuid machine.UUID) (network.SpaceAddress, error)
}

// ApplicationService is the interface that is used to interact with the
// applications.
type ApplicationService interface {
	// GetUnitMachineName gets the name of the unit's machine.
	//
	// The following errors may be returned:
	//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
	GetUnitMachineName(context.Context, unit.Name) (machine.Name, error)
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// ModelProviderService providers access to the model provider service.
type ModelProviderService interface {
	// GetCloudSpecForSSH returns the cloud spec for sshing into a k8s pod.
	GetCloudSpecForSSH(ctx context.Context) (cloudspec.CloudSpec, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns an errors.MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error)
}
