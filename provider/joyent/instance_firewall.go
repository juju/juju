// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"launchpad.net/juju-core/instance"
)

// Implementing the methods below (to do something other than return nil) will
// cause `juju expose` to work when the firewall-mode is "instance". If you
// implement one of them, you should implement them all.

func (inst *joyentInstance) OpenPorts(machineId string, ports []instance.Port) error {
	logger.Warningf("pretending to open ports %v for instance %q", ports, inst.machine.Id)
	return nil
}

func (inst *joyentInstance) ClosePorts(machineId string, ports []instance.Port) error {
	logger.Warningf("pretending to close ports %v for instance %q", ports, inst.machine.Id)
	return nil
}

func (inst *joyentInstance) Ports(machineId string) ([]instance.Port, error) {
	return nil, nil
}
