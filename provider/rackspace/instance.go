// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
<<<<<<< HEAD
<<<<<<< HEAD
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
=======
=======
	"github.com/juju/errors"

>>>>>>> working version of rackspace provider
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

type environInstance struct {
	openstackInstance instance.Instance
}

// Id implements instance.Instance.
func (i environInstance) Id() instance.Id {
	return i.openstackInstance.Id()
}

// Status implements instance.Instance.
func (i environInstance) Status() string {
	return i.openstackInstance.Status()
}

// Refresh implements instance.Instance.
func (i environInstance) Refresh() error {
	return i.openstackInstance.Refresh()
}

// Addresses implements instance.Instance.
func (i environInstance) Addresses() ([]network.Address, error) {
	return i.openstackInstance.Addresses()
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
<<<<<<< HEAD
	return i.openstackInstance.Ports(machineId)
>>>>>>> modifications to opestack provider applied
=======
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
>>>>>>> working version of rackspace provider
}
