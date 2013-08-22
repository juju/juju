// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretools "launchpad.net/juju-core/tools"
)

// StartInstance uses the supplied broker to start a machine instance.
func StartInstance(broker environs.Broker, machineId, machineNonce string, series string, cons constraints.Value,
	stateInfo *state.Info, apiInfo *api.Info) (instance.Instance, *instance.HardwareCharacteristics, error) {

	var err error
	var possibleTools coretools.List
	if env, ok := broker.(environs.Environ); ok {
		possibleTools, err = tools.FindInstanceTools(env, series, cons)
		if err != nil {
			return nil, nil, err
		}
	} else if hasTools, ok := broker.(coretools.HasTools); ok {
		possibleTools = hasTools.Tools()
	} else {
		panic(fmt.Errorf("broker of type %T does not provide any tools", broker))
	}
	machineConfig := environs.NewMachineConfig(machineId, machineNonce, stateInfo, apiInfo)
	return broker.StartInstance(cons, possibleTools, machineConfig)
}
