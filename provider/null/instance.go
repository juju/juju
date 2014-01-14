// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/instance"
)

type nullBootstrapInstance struct {
	host string
}

func (nullBootstrapInstance) Id() instance.Id {
	// The only way to bootrap is via manual bootstrap.
	return manual.BootstrapInstanceId
}

func (nullBootstrapInstance) Status() string {
	return ""
}

func (nullBootstrapInstance) Refresh() error {
	return nil
}

func (inst nullBootstrapInstance) Addresses() (addresses []instance.Address, err error) {
	host, err := inst.DNSName()
	if err != nil {
		return nil, err
	}
	addresses, err = instance.HostAddresses(host)
	if err != nil {
		return nil, err
	}
	// Add a HostName type address.
	addresses = append(addresses, instance.NewAddress(host))
	return addresses, nil
}

func (inst nullBootstrapInstance) DNSName() (string, error) {
	return inst.host, nil
}

func (i nullBootstrapInstance) WaitDNSName() (string, error) {
	return i.DNSName()
}

func (nullBootstrapInstance) OpenPorts(machineId string, ports []instance.Port) error {
	return nil
}

func (nullBootstrapInstance) ClosePorts(machineId string, ports []instance.Port) error {
	return nil
}

func (nullBootstrapInstance) Ports(machineId string) ([]instance.Port, error) {
	return []instance.Port{}, nil
}
