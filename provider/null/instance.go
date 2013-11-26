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

func (_ nullBootstrapInstance) Id() instance.Id {
	// The only way to bootrap is via manual bootstrap.
	return manual.BootstrapInstanceId
}

func (_ nullBootstrapInstance) Status() string {
	return ""
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
	// If the user specified bootstrap-host as an IP address,
	// do a reverse lookup.
	host := inst.host
	return host, nil
}

func (i nullBootstrapInstance) WaitDNSName() (string, error) {
	return i.DNSName()
}

func (_ nullBootstrapInstance) OpenPorts(machineId string, ports []instance.Port) error {
	return nil
}

func (_ nullBootstrapInstance) ClosePorts(machineId string, ports []instance.Port) error {
	return nil
}

func (_ nullBootstrapInstance) Ports(machineId string) ([]instance.Port, error) {
	return []instance.Port{}, nil
}
