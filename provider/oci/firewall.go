// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

func (e *Environ) OpenPorts(rules []network.IngressRule) error {
	return errors.NotImplementedf("OpenPorts")
}

func (e *Environ) ClosePorts(rules []network.IngressRule) error {
	return errors.NotImplementedf("ClosePorts")
}

func (e *Environ) IngressRules() ([]network.IngressRule, error) {
	return nil, errors.NotImplementedf("IngressRules")
}
