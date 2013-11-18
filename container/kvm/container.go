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
	started *bool
}

var _ Container = (*kvmContainer)(nil)

func (c *kvmContainer) Name() string {
	return c.name
}

func (c *kvmContainer) Start(
	hostname string,
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
	bridge := ""
	if network != nil {
		if network.NetworkType == container.BridgeNetwork {
			bridge = network.Device
		} else {
			return log.LoggedErrorf("Non-bridge network devices not yet supported")
		}
	}
	logger.Debugf("Create the machine %s", hostname)
	if err := CreateMachine(CreateMachineParams{
		Hostname:      hostname,
		Series:        series,
		Arch:          arch,
		UserData:      userDataFile,
		NetworkBridge: bridge,
	}); err != nil {
		return err
	}

	logger.Debugf("Set machine %s to autostart", hostname)
	return AutostartMachine(c.name)
}

func (c *kvmContainer) Stop() error {
	if c.IsRunning() {
		// Make started state unknown again.
		c.started = nil
		logger.Debugf("Stop %s", c.name)
		return DestroyMachine(c.name)
	}
	logger.Debugf("%s is already stopped", c.name)
	return nil
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
