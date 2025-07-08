// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"errors"

	"github.com/juju/collections/transform"

	corenetwork "github.com/juju/juju/core/network"
)

// ImportLinkLayerDevice represents a physical or virtual
// network interface and its IP addresses.
type ImportLinkLayerDevice struct {
	UUID             string
	IsAutoStart      bool
	IsEnabled        bool
	MTU              *int64
	MachineID        string
	MACAddress       *string
	NetNodeUUID      string
	Name             string
	ParentDeviceName string
	ProviderID       *string
	Type             corenetwork.LinkLayerDeviceType
	VirtualPortType  corenetwork.VirtualPortType
}

// SpaceName represents a space's name and its unique identifier.
type SpaceName struct {
	// UUID is the unique identifier for the space.
	UUID string
	// Name is the human-readable name of the space.
	Name string
}

// CheckableMachine is used to validate machines against a new network topology.
type CheckableMachine interface {
	// Accept processes a given collection of SpaceInfos and returns an error
	// if the machine isn't acceptable in the new topology
	Accept(topology corenetwork.SpaceInfos) error
}

// CheckableMachines represents a collection of CheckableMachine instances
// providing batch topology validation.
type CheckableMachines []CheckableMachine

// Accept validates a collection of machines against the provided topology
func (m CheckableMachines) Accept(topology corenetwork.SpaceInfos) error {
	return errors.Join(transform.Slice(m, func(machine CheckableMachine) error { return machine.Accept(topology) })...)
}
