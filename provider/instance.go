// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	coretools "launchpad.net/juju-core/tools"
)

var logger = loggo.GetLogger("juju.provider")

// StartInstance uses the supplied broker to start a machine instance.
func StartInstance(broker environs.InstanceBroker, machineId, machineNonce string, series string, cons constraints.Value,
	stateInfo *state.Info, apiInfo *api.Info) (instance.Instance, *instance.HardwareCharacteristics, error) {

	var err error
	var possibleTools coretools.List
	if env, ok := broker.(environs.Environ); ok {
		agentVersion, ok := env.Config().AgentVersion()
		if !ok {
			return nil, nil, fmt.Errorf("no agent version set in environment configuration")
		}
		possibleTools, err = tools.FindInstanceTools(env, agentVersion, series, cons.Arch)
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

// StartBootstrapInstance starts the bootstrap instance for the environment.
func StartBootstrapInstance(env environs.Environ, cons constraints.Value, possibleTools coretools.List, machineID string) error {

	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	// Create an empty bootstrap state file so we can get its URL.
	// It will be updated with the instance id and hardware characteristics
	// after the bootstrap instance is started.
	stateFileURL, err := CreateStateFile(env.Storage())
	if err != nil {
		return err
	}
	machineConfig := environs.NewBootstrapMachineConfig(machineID, stateFileURL)
	inst, hw, err := env.StartInstance(cons, possibleTools, machineConfig)
	if err != nil {
		return fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	var characteristics []instance.HardwareCharacteristics
	if hw != nil {
		characteristics = []instance.HardwareCharacteristics{*hw}
	}
	err = SaveState(
		env.Storage(),
		&BootstrapState{
			StateInstances:  []instance.Id{inst.Id()},
			Characteristics: characteristics,
		})
	if err != nil {
		stoperr := env.StopInstances([]instance.Instance{inst})
		if stoperr != nil {
			// Failure upon failure.  Log it, but return the original error.
			logger.Errorf("cannot release failed bootstrap instance: %v", stoperr)
		}
		return fmt.Errorf("cannot save state: %v", err)
	}
	return nil
}
