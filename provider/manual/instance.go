// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/instance"
)

type manualBootstrapInstance struct {
	host string
}

func (manualBootstrapInstance) Id() instance.Id {
	// The only way to bootrap is via manual bootstrap.
	return manual.BootstrapInstanceId
}

func (manualBootstrapInstance) Status() string {
	return ""
}

func (manualBootstrapInstance) Refresh() error {
	return nil
}

func (inst manualBootstrapInstance) Addresses() (addresses []instance.Address, err error) {
	return manual.HostAddresses(inst.host)
}

func (inst manualBootstrapInstance) DNSName() (string, error) {
	return inst.host, nil
}

func (i manualBootstrapInstance) WaitDNSName() (string, error) {
	return i.DNSName()
}

func (manualBootstrapInstance) OpenPorts(machineId string, ports []instance.Port) error {
	return nil
}

func (manualBootstrapInstance) ClosePorts(machineId string, ports []instance.Port) error {
	return nil
}

func (manualBootstrapInstance) Ports(machineId string) ([]instance.Port, error) {
	return []instance.Port{}, nil
}
