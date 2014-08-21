// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type manualBootstrapInstance struct {
	host string
}

func (manualBootstrapInstance) Id() instance.Id {
	return BootstrapInstanceId
}

func (manualBootstrapInstance) Status() string {
	return ""
}

func (manualBootstrapInstance) Refresh() error {
	return nil
}

func (inst manualBootstrapInstance) Addresses() (addresses []network.Address, err error) {
	addr, err := manual.HostAddress(inst.host)
	if err != nil {
		return nil, err
	}
	return []network.Address{addr}, nil
}

func (manualBootstrapInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	return nil
}

func (manualBootstrapInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	return nil
}

func (manualBootstrapInstance) Ports(machineId string) ([]network.PortRange, error) {
	return nil, nil
}
