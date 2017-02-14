// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import "github.com/juju/juju/network"

// Implementing the methods below (to do something other than return nil) will
// cause `juju expose` to work when the firewall-mode is "global". If you
// implement one of them, you should implement them all.

// OpenPorts opens the given ports for the whole environment.
// Must only be used if the environment was setup with the FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.IngressRule) error {
	logger.Debugf("pretending to open ports %v for all instances", ports)
	return nil
}

// ClosePorts closes the given ports for the whole environment.
// Must only be used if the environment was setup with the FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.IngressRule) error {
	logger.Debugf("pretending to close ports %v for all instances", ports)
	return nil
}

// IngressRules returns the ports opened for the whole environment.
// Must only be used if the environment was setup with the FwGlobal firewall mode.
func (env *environ) IngressRules() ([]network.IngressRule, error) {
	return nil, nil
}
