// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/network/firewall"
)

// OpenPorts is part of the environs.Firewaller interface.
func (*environ) OpenPorts(rules firewall.IngressRules) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// ClosePorts is part of the environs.Firewaller interface.
func (*environ) ClosePorts(rules firewall.IngressRules) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// IngressPorts is part of the environs.Firewaller interface.
func (*environ) IngressRules() (firewall.IngressRules, error) {
	return nil, errors.Trace(errors.NotSupportedf("Ports"))
}
