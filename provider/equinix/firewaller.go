// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

type equinixFirewaller struct{}

var _ environs.Firewaller = (*equinixFirewaller)(nil)

// OpenPorts is not supported.
func (c *equinixFirewaller) OpenPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	return errors.NotSupportedf("OpenPorts")
}

// ClosePorts is not supported.
func (c *equinixFirewaller) ClosePorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	return errors.NotSupportedf("ClosePorts")
}

// IngressRules returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (c *equinixFirewaller) IngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error) {
	return nil, errors.NotSupportedf("Ports")
}
