// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/tools"
)

// TODO(wallyworld) - we want this in the environs/instance package but import loops
// stop that from being possible right now.
type InstanceBroker interface {
	// StartInstance asks for a new instance to be created, associated with
	// the provided config in machineConfig. The given config describes the juju
	// state for the new instance to connect to. The config MachineNonce, which must be
	// unique within an environment, is used by juju to protect against the
	// consequences of multiple instances being started with the same machine
	// id.
	StartInstance(
		cons constraints.Value, possibleTools tools.List,
		machineConfig *cloudinit.MachineConfig,
	) (instance.Instance, *instance.HardwareCharacteristics, error)

	// StopInstances shuts down the given instances.
	StopInstances([]instance.Instance) error

	// AllInstances returns all instances currently known to the broker.
	AllInstances() ([]instance.Instance, error)
}
