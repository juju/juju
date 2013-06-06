// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

type Broker interface {
	// StartInstance asks for a new instance to be created, associated with
	// the provided machine identifier. The given info describes the juju
	// state for the new instance to connect to. The nonce, which must be
	// unique within an environment, is used by juju to protect against the
	// consequences of multiple instances being started with the same machine
	// id.
	StartInstance(machineId, machineNonce string, series string, cons constraints.Value, info *state.Info, apiInfo *api.Info) (environs.Instance, error)

	// StopInstances shuts down the given instances.
	StopInstances([]environs.Instance) error

	// AllInstances returns all instances currently known to the broker.
	AllInstances() ([]environs.Instance, error)

	// AllMachines returns all the machines in state that relate to this broker.
	AllMachines() ([]*state.Machine, error)
}
