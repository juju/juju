// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"

	"launchpad.net/gwacl"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
)

type azureInstance struct {
	// An instance contains an Azure Service (instance==service).
	gwacl.HostedServiceDescriptor
}

// azureInstance implements Instance.
var _ instance.Instance = (*azureInstance)(nil)

// Id is specified in the Instance interface.
func (azInstance *azureInstance) Id() instance.Id {
	return instance.Id(azInstance.ServiceName)
}

var AZURE_DOMAIN_NAME = "cloudapp.net"

// DNSName is specified in the Instance interface.
func (azInstance *azureInstance) DNSName() (string, error) {
	// For deployments in the Production slot, the instance's DNS name
	// is its service name, in the cloudapp.net domain.
	// (For Staging deployments it's all much weirder: they get random
	// names assigned, which somehow don't seem to resolve from the
	// outside.)
	name := fmt.Sprintf("%s.%s", azInstance.ServiceName, AZURE_DOMAIN_NAME)
	return name, nil
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
