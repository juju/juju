// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
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
	StartInstance(machineId, machineNonce string, series string, cons constraints.Value,
		info *state.Info, apiInfo *api.Info) (instance.Instance, *instance.HardwareCharacteristics, error)

	// StopInstances shuts down the given instances.
	StopInstances([]instance.Instance) error

	// AllInstances returns all instances currently known to the broker.
	AllInstances() ([]instance.Instance, error)
}
