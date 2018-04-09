// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type ociInstance struct{}

var _ instance.Instance = (*ociInstance)(nil)
var _ instance.InstanceFirewaller = (*ociInstance)(nil)

// Id implements instance.Instance
func (o *ociInstance) Id() instance.Id {
	return instance.Id(0)
}

// Status implements instance.Instance
func (o *ociInstance) Status() instance.InstanceStatus {
	return instance.InstanceStatus{}
}

// Addresses implements instance.Instance
func (o *ociInstance) Addresses() ([]network.Address, error) {
	return nil, errors.NotImplementedf("Addresses")
}

// OpenPorts implements instance.InstanceFirewaller
func (o *ociInstance) OpenPorts(machineId string, rules []network.IngressRule) error {
	return errors.NotImplementedf("OpenPorts")
}

// ClosePorts implements instance.InstanceFirewaller
func (o *ociInstance) ClosePorts(machineId string, rules []network.IngressRule) error {
	return errors.NotImplementedf("ClosePorts")
}

// IngressRules implements instance.InstanceFirewaller
func (o *ociInstance) IngressRules(machineId string) ([]network.IngressRule, error) {
	return nil, errors.NotImplementedf("IngressRules")
}
