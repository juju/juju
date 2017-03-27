// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(rules []network.IngressRule) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(rules []network.IngressRule) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// IngressRules returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) IngressRules() ([]network.IngressRule, error) {
	return nil, errors.Trace(errors.NotSupportedf("Ports"))
}
