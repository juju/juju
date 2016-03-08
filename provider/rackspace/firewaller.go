// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"
	"gopkg.in/goose.v1/nova"

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

// InitialNetworks implements Firewaller interface.
func (c *rackspaceFirewaller) InitialNetworks() []nova.ServerNetworks {
	// These are the default rackspace networks, see:
	// http://docs.rackspace.com/servers/api/v2/cs-devguide/content/provision_server_with_networks.html
	return []nova.ServerNetworks{
		{NetworkId: "00000000-0000-0000-0000-000000000000"}, //Racksapce PublicNet
		{NetworkId: "11111111-1111-1111-1111-111111111111"}, //Rackspace ServiceNet
	}
}

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

// DeleteGlobalGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) DeleteGlobalGroups() error {
	return nil
}

// GetSecurityGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) GetSecurityGroups(ids ...instance.Id) ([]string, error) {
	return nil, nil
}

// SetUpGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) SetUpGroups(machineId string, apiPort int) ([]nova.SecurityGroup, error) {
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
