// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
)

// OpenPorts is part of the environs.Firewaller interface.
func (*environ) OpenPorts(_ firewall.IngressRules) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// ClosePorts is part of the environs.Firewaller interface.
func (*environ) ClosePorts(_ firewall.IngressRules) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// IngressRules is part of the environs.Firewaller interface.
func (*environ) IngressRules() (firewall.IngressRules, error) {
	return nil, errors.Trace(errors.NotSupportedf("Ports"))
}

// NetworkInterfaces exists only to satisfy the Environ indirection of the
// instance-poller worker.
// Without this method the worker throws an error, which prevents the status of
// instances being known.
// The provider does not support the AllocatePublicIP constraint, which means
// we can assume that the network configuration returned by the machine agent
// represents the knowable link-layer data for each VM - there is no
// elaboration available from the provider.
func (env *environ) NetworkInterfaces(_ context.Context, _ []instance.Id) ([]network.InterfaceInfos, error) {
	return nil, environs.ErrNoInstances
}
