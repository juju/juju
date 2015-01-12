// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
)

// StartInstanceParams holds parameters for the
// InstanceBroker.StartInstance method.
type StartInstanceParams struct {
	// Constraints is a set of constraints on
	// the kind of instance to create.
	Constraints constraints.Value

	// Tools is a list of tools that may be used
	// to start a Juju agent on the machine.
	Tools tools.List

	// MachineConfig describes the machine's configuration.
	MachineConfig *cloudinit.MachineConfig

	// Placement, if non-empty, contains an environment-specific
	// placement directive that may be used to decide how the
	// instance should be started.
	Placement string

	// DistributionGroup, if non-nil, is a function
	// that returns a slice of instance.Ids that belong
	// to the same distribution group as the machine
	// being provisioned. The InstanceBroker may use
	// this information to distribute instances for
	// high availability.
	DistributionGroup func() ([]instance.Id, error)

	// Disks is a set of parameters for disks that must be created.
	// If any of the disks cannot be created, StartInstance must
	// return an error.
	Disks []storage.DiskParams
}

// StartInstanceResult holds the result of an
// InstanceBroker.StartInstance method call.
type StartInstanceResult struct {
	// Instance is an interface representing a cloud instance.
	Instance instance.Instance

	// HardwareCharacteristics represents the hardware characteristics
	// of the newly created instance.
	Hardware *instance.HardwareCharacteristics

	// NetworkInfo contains information about configured networks.
	NetworkInfo []network.Info

	// Disks contains a list of block devices created, each one having
	// the same Name as one of the DiskParams in StartInstanceParams.Disks.
	Disks []storage.BlockDevice
}

// TODO(wallyworld) - we want this in the environs/instance package but import loops
// stop that from being possible right now.
type InstanceBroker interface {
	// StartInstance asks for a new instance to be created, associated with
	// the provided config in machineConfig. The given config describes the juju
	// state for the new instance to connect to. The config MachineNonce, which must be
	// unique within an environment, is used by juju to protect against the
	// consequences of multiple instances being started with the same machine
	// id.
	StartInstance(args StartInstanceParams) (*StartInstanceResult, error)

	// StopInstances shuts down the instances with the specified IDs.
	// Unknown instance IDs are ignored, to enable idempotency.
	StopInstances(...instance.Id) error

	// AllInstances returns all instances currently known to the broker.
	AllInstances() ([]instance.Instance, error)
}
