// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"launchpad.net/juju-core/instance"
)

type azureInstance struct{}

// azureInstance implements Instance.
var _ instance.Instance = (*azureInstance)(nil)

// Id is specified in the Instance interface.
func (instance *azureInstance) Id() instance.Id {
	panic("unimplemented")
}

// DNSName is specified in the Instance interface.
func (instance *azureInstance) DNSName() (string, error) {
	panic("unimplemented")
}

// WaitDNSName is specified in the Instance interface.
func (instance *azureInstance) WaitDNSName() (string, error) {
	panic("unimplemented")
}

// OpenPorts is specified in the Instance interface.
func (instance *azureInstance) OpenPorts(machineId string, ports []instance.Port) error {
	panic("unimplemented")
}

// ClosePorts is specified in the Instance interface.
func (instance *azureInstance) ClosePorts(machineId string, ports []instance.Port) error {
	panic("unimplemented")
}

// Ports is specified in the Instance interface.
func (instance *azureInstance) Ports(machineId string) ([]instance.Port, error) {
	panic("unimplemented")
}

// Metadata is specified in the Instance interface.
func (instance *azureInstance) Metadata() (*instance.Metadata, error) {
	panic("unimplemented")
}
