// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/instance"
	coretools "launchpad.net/juju-core/tools"
)

var logger = loggo.GetLogger("juju.provider.common")

// Bootstrap is a common implementation of the Bootstrap method defined on
// environs.Environ; we strongly recommend that this implementation be used
// when writing a new provider.
func Bootstrap(env environs.Environ, cons constraints.Value) error {
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	// Check to see if the environment is already bootstrapped
	// before potentially uploading any tools.
	if err := EnsureNotBootstrapped(env); err != nil {
		return err
	}

	// Create an empty bootstrap state file so we can get its URL.
	// It will be updated with the instance id and hardware characteristics
	// after the bootstrap instance is started.
	stateFileURL, err := CreateStateFile(env.Storage())
	if err != nil {
		return err
	}
	machineConfig := environs.NewBootstrapMachineConfig(stateFileURL)

	selectedTools, err := SetBootstrapTools(env, env.Config().DefaultSeries(), cons.Arch)
	if err != nil {
		return err
	}

	inst, hw, err := env.StartInstance(cons, selectedTools, machineConfig)
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
			logger.Errorf("cannot stop failed bootstrap instance %q: %v", inst.Id(), stoperr)
		}
		return fmt.Errorf("cannot save state: %v", err)
	}
	return nil
}

// SetBootstrapTools finds tools, syncing with an external tools source as
// necessary; it then selects the newest tools to bootstrap with, and sets
// agent-version.
func SetBootstrapTools(env environs.Environ, series string, arch *string) (coretools.List, error) {
	possibleTools, err := bootstrap.EnsureToolsAvailability(env, series, arch)
	if err != nil {
		return nil, err
	}
	return bootstrap.SelectBootstrapTools(env, possibleTools)
}

// EnsureNotBootstrapped returns null if the environment is not bootstrapped,
// and an error if it is or if the function was not able to tell.
func EnsureNotBootstrapped(env environs.Environ) error {
	_, err := LoadState(env.Storage())
	// If there is no error loading the bootstrap state, then we are
	// bootstrapped.
	if err == nil {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if err == environs.ErrNotBootstrapped {
		return nil
	}
	return err
}
