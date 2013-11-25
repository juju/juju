// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/log"
)

type kvmContainer struct {
	factory *containerFactory
	name    string
	// started is a three state boolean, true, false, or unknown
	// this allows for checking when we don't know, but using a
	// value if we already know it (like in the list situation).
	started *bool
}

var _ Container = (*kvmContainer)(nil)

func (c *kvmContainer) Name() string {
	return c.name
}

func (c *kvmContainer) Start(
	series string,
	arch string,
	userDataFile string,
	network *container.NetworkConfig,
) error {
	// TODO: handle memory, cpu, disk etc.
	logger.Debugf("Synchronise images for %s %s", series, arch)
	if err := SyncImages(series, arch); err != nil {
		return err
	}
	var bridge string
	if network != nil {
		if network.NetworkType == container.BridgeNetwork {
			bridge = network.Device
		} else {
			return log.LoggedErrorf(logger, "Non-bridge network devices not yet supported")
		}
	}
	logger.Debugf("Create the machine %s", c.name)
	if err := CreateMachine(CreateMachineParams{
		Hostname:      c.name,
		Series:        series,
		Arch:          arch,
		UserDataFile:  userDataFile,
		NetworkBridge: bridge,
	}); err != nil {
		return err
	}

	logger.Debugf("Set machine %s to autostart", c.name)
	return AutostartMachine(c.name)
}

func (c *kvmContainer) Stop() error {
	if !c.IsRunning() {
		logger.Debugf("%s is already stopped", c.name)
		return nil
	}
	// Make started state unknown again.
	c.started = nil
	logger.Debugf("Stop %s", c.name)
	return DestroyMachine(c.name)
}

func (c *kvmContainer) IsRunning() bool {
	if c.started != nil {
		return *c.started
	}
	machines, err := ListMachines()
	if err != nil {
		return false
	}
	c.started = isRunning(machines[c.name])
	return *c.started
}

func (c *kvmContainer) String() string {
	return fmt.Sprintf("<KVM container %v>", *c)
}
