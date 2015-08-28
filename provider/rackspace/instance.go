// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

type environInstance struct {
	instance.Instance
}

// OpenPorts implements instance.Instance.
func (i environInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	return i.changePorts(true, ports)
}

// ClosePorts implements instance.Instance.
func (i environInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	return i.changePorts(false, ports)
}

// Ports implements instance.Instance.
func (i environInstance) Ports(machineId string) ([]network.PortRange, error) {
	_, configurator, err := i.getSshInstanceConfigurator()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return configurator.FindOpenPorts()
}

func (i environInstance) changePorts(insert bool, ports []network.PortRange) error {
	addresses, sshClient, err := i.getSshInstanceConfigurator()
	if err != nil {
		return errors.Trace(err)
	}

	for _, addr := range addresses {
		if addr.Scope == network.ScopePublic {
			err = sshClient.ChangePorts(addr.Value, insert, ports)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (i environInstance) getSshInstanceConfigurator() ([]network.Address, *common.SshInstanceConfigurator, error) {
	addresses, err := i.Addresses()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if len(addresses) == 0 {
		return addresses, nil, errors.New("No addresses found")
	}

	client := common.NewSshInstanceConfigurator(addresses[0].Value)
	return addresses, client, err
}
