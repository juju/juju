// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	return nil, errors.Trace(errors.NotSupportedf("Ports"))
}
