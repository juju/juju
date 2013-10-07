// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"launchpad.net/juju-core/instance"
)

// Implementing the methods below (to do something other than return nil) will
// cause `juju expose` to work when the firewall-mode is "global". If you
// implement one of them, you should implement them all.

func (env *environ) OpenPorts(ports []instance.Port) error {
	logger.Warningf("pretending to open ports %v for all instances", ports)
	_ = env.getSnapshot()
	return nil
}

func (env *environ) ClosePorts(ports []instance.Port) error {
	logger.Warningf("pretending to close ports %v for all instances", ports)
	_ = env.getSnapshot()
	return nil
}

func (env *environ) Ports() ([]instance.Port, error) {
	_ = env.getSnapshot()
	return nil, nil
}
