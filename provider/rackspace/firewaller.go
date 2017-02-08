// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/openstack"
)

type firewallerFactory struct {
}

var _ openstack.FirewallerFactory = (*firewallerFactory)(nil)

// GetFirewaller implements FirewallerFactory
func (f *firewallerFactory) GetFirewaller(env environs.Environ) openstack.Firewaller {
	return &rackspaceFirewaller{}
}

type rackspaceFirewaller struct{}

var _ openstack.Firewaller = (*rackspaceFirewaller)(nil)

// OpenPorts is not supported.
func (c *rackspaceFirewaller) OpenPorts(ports []network.PortRange) error {
	return errors.NotSupportedf("OpenPorts")
}

// ClosePorts is not supported.
func (c *rackspaceFirewaller) ClosePorts(ports []network.PortRange) error {
	return errors.NotSupportedf("ClosePorts")
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (c *rackspaceFirewaller) Ports() ([]network.PortRange, error) {
	return nil, errors.NotSupportedf("Ports")
}

// DeleteGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) DeleteGroups(names ...string) error {
	return nil
}

// DeleteAllModelGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) DeleteAllModelGroups() error {
	return nil
}

// DeleteAllControllerGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) DeleteAllControllerGroups(controllerUUID string) error {
	return nil
}

func (c *rackspaceFirewaller) UpdateGroupController(controllerUUID string) error {
	return nil
}

// GetSecurityGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) GetSecurityGroups(ids ...instance.Id) ([]string, error) {
	return nil, nil
}

// SetUpGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) SetUpGroups(controllerUUID, machineId string, apiPort int) ([]string, error) {
	return nil, nil
}

// OpenInstancePorts implements Firewaller interface.
func (c *rackspaceFirewaller) OpenInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error {
	return c.changePorts(inst, true, ports)
}

// CloseInstancePorts implements Firewaller interface.
func (c *rackspaceFirewaller) CloseInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error {
	return c.changePorts(inst, false, ports)
}

// InstancePorts implements Firewaller interface.
func (c *rackspaceFirewaller) InstancePorts(inst instance.Instance, machineId string) ([]network.PortRange, error) {
	_, configurator, err := c.getInstanceConfigurator(inst)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return configurator.FindOpenPorts()
}

func (c *rackspaceFirewaller) changePorts(inst instance.Instance, insert bool, ports []network.PortRange) error {
	addresses, sshClient, err := c.getInstanceConfigurator(inst)
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

func (c *rackspaceFirewaller) getInstanceConfigurator(inst instance.Instance) ([]network.Address, common.InstanceConfigurator, error) {
	addresses, err := inst.Addresses()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if len(addresses) == 0 {
		return addresses, nil, errors.New("No addresses found")
	}

	client := common.NewSshInstanceConfigurator(addresses[0].Value)
	return addresses, client, err
}
