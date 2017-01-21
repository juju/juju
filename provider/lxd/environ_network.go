// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

// globalFirewallName returns the name to use for the global firewall.
func (env *environ) globalFirewallName() string {
	return common.EnvFullName(env.uuid)
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(rules []network.IngressRule) error {
	err := env.raw.OpenPorts(env.globalFirewallName(), rules...)
	if errors.IsNotImplemented(err) {
		// TODO(ericsnow) for now...
		return nil
	}
	return errors.Trace(err)
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(rules []network.IngressRule) error {
	err := env.raw.ClosePorts(env.globalFirewallName(), rules...)
	if errors.IsNotImplemented(err) {
		// TODO(ericsnow) for now...
		return nil
	}
	return errors.Trace(err)
}

// IngressRules returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) IngressRules() ([]network.IngressRule, error) {
	ports, err := env.raw.IngressRules(env.globalFirewallName())
	if errors.IsNotImplemented(err) {
		// TODO(ericsnow) for now...
		return nil, nil
	}
	return ports, errors.Trace(err)
}
