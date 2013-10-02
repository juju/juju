// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	coretools "launchpad.net/juju-core/tools"
)

var logger = loggo.GetLogger("juju.provider.common")

// Bootstrap is a common implementation of the Bootstrap method defined on
// environs.Environ; we strongly recommend that this implementation be used
// when writing a new provider.
func Bootstrap(env environs.Environ, cons constraints.Value, possibleTools coretools.List, machineID string) error {
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
