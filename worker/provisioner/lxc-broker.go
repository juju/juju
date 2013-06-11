// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

func newLxcBroker() Broker {
	return &lxcBroker{}
}

type lxcBroker struct {
}

func (broker *lxcBroker) StartInstance(machineId, machineNonce string, series string, cons constraints.Value, info *state.Info, apiInfo *api.Info) (environs.Instance, error) {

	return nil, fmt.Errorf("Not implemented yet")
}

// StopInstances shuts down the given instances.
func (broker *lxcBroker) StopInstances([]environs.Instance) error {
	return fmt.Errorf("Not implemented yet")
}

func (broker *lxcBroker) AllInstances() ([]environs.Instance, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
