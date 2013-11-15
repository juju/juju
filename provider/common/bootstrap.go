// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"net"
	"time"

	"launchpad.net/loggo"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/cloudinit/sshinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	coretools "launchpad.net/juju-core/tools"
)

var logger = loggo.GetLogger("juju.provider.common")

// Bootstrap is a common implementation of the Bootstrap method defined on
// environs.Environ; we strongly recommend that this implementation be used
// when writing a new provider.
func Bootstrap(env environs.Environ, cons constraints.Value, possibleTools coretools.List) (err error) {
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	var inst instance.Instance
	defer func() {
		if err == nil || inst == nil {
			return
		}
		if stoperr := env.StopInstances([]instance.Instance{inst}); stoperr != nil {
			// Failure upon failure.  Log it, but return the original error.
			logger.Errorf("cannot stop failed bootstrap instance %q: %v", inst.Id(), stoperr)
		}
	}()

	// Create an empty bootstrap state file so we can get its URL.
	// It will be updated with the instance id and hardware characteristics
	// after the bootstrap instance is started.
	stateFileURL, err := CreateStateFile(env.Storage())
	if err != nil {
		return err
	}
	machineConfig := environs.NewBootstrapMachineConfig(stateFileURL)
	var hw *instance.HardwareCharacteristics
	inst, hw, err = env.StartInstance(cons, possibleTools, machineConfig)
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
		return fmt.Errorf("cannot save state: %v", err)
	}
	logger.Infof("waiting for DNS name for instance %q", inst.Id())
	dnsName, err := WaitDNSName(inst)
	if err != nil {
		return err
	}
	// Wait until we can open a connection to port 22.
	connected := false
	for a := LongAttempt.Start(); !connected && a.Next(); {
		logger.Infof("attempting to connect to %s:22...", dnsName)
		conn, err := net.DialTimeout("tcp", dnsName+":22", 5*time.Second)
		if err == nil {
			conn.Close()
			connected = true
		} else {
			logger.Errorf("failed to connect: %v", err)
		}
	}
	if !connected {
		return fmt.Errorf("could not connect to host")
	}
	cloudcfg := coreCloudinit.New()
	if err := cloudinit.ConfigureJuju(machineConfig, cloudcfg); err != nil {
		return err
	}
	return sshinit.Configure("ubuntu@"+dnsName, cloudcfg)
}
