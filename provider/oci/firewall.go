// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs/context"
)

func (e *Environ) OpenPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	return errors.NotImplementedf("OpenPorts")
}

func (e *Environ) ClosePorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	return errors.NotImplementedf("ClosePorts")
}

func (e *Environ) IngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error) {
	return nil, errors.NotImplementedf("IngressRules")
}
