// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
)

func (e *Environ) OpenPorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return errors.NotImplementedf("OpenPorts")
}

func (e *Environ) ClosePorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return errors.NotImplementedf("ClosePorts")
}

func (e *Environ) IngressRules(ctx context.ProviderCallContext) ([]network.IngressRule, error) {
	return nil, errors.NotImplementedf("IngressRules")
}
