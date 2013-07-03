// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
)

type azureInstance struct {
	gwacl.Deployment
}

// azureInstance implements Instance.
var _ instance.Instance = (*azureInstance)(nil)

// Id is specified in the Instance interface.
func (azInstance *azureInstance) Id() instance.Id {
	return instance.Id(azInstance.Name)
}

// DNSName is specified in the Instance interface.
func (azInstance *azureInstance) DNSName() (string, error) {
	// An Azure deployment gets its DNS name immediately when it's created.
	return azInstance.GetFQDN()
}

// WaitDNSName is specified in the Instance interface.
func (azInstance *azureInstance) WaitDNSName() (string, error) {
	return environs.WaitDNSName(azInstance)
}

// OpenPorts is specified in the Instance interface.
func (azInstance *azureInstance) OpenPorts(machineId string, ports []instance.Port) error {
	panic("unimplemented")
}

// ClosePorts is specified in the Instance interface.
func (azInstance *azureInstance) ClosePorts(machineId string, ports []instance.Port) error {
	panic("unimplemented")
}

// Ports is specified in the Instance interface.
func (azInstance *azureInstance) Ports(machineId string) ([]instance.Port, error) {
	panic("unimplemented")
}
