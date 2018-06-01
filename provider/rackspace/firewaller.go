// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
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
func (c *rackspaceFirewaller) OpenPorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return errors.NotSupportedf("OpenPorts")
}

// ClosePorts is not supported.
func (c *rackspaceFirewaller) ClosePorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return errors.NotSupportedf("ClosePorts")
}

// IngressRules returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (c *rackspaceFirewaller) IngressRules(ctx context.ProviderCallContext) ([]network.IngressRule, error) {
	return nil, errors.NotSupportedf("Ports")
}

// DeleteGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) DeleteGroups(ctx context.ProviderCallContext, names ...string) error {
	return nil
}

// DeleteAllModelGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) DeleteAllModelGroups(ctx context.ProviderCallContext) error {
	return nil
}

// DeleteAllControllerGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) DeleteAllControllerGroups(ctx context.ProviderCallContext, controllerUUID string) error {
	return nil
}

func (c *rackspaceFirewaller) UpdateGroupController(ctx context.ProviderCallContext, controllerUUID string) error {
	return nil
}

// GetSecurityGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) GetSecurityGroups(ctx context.ProviderCallContext, ids ...instance.Id) ([]string, error) {
	return nil, nil
}

// SetUpGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) SetUpGroups(ctx context.ProviderCallContext, controllerUUID, machineId string, apiPort int) ([]string, error) {
	return nil, nil
}

// OpenInstancePorts implements Firewaller interface.
func (c *rackspaceFirewaller) OpenInstancePorts(ctx context.ProviderCallContext, inst instance.Instance, machineId string, rules []network.IngressRule) error {
	return c.changeIngressRules(ctx, inst, true, rules)
}

// CloseInstancePorts implements Firewaller interface.
func (c *rackspaceFirewaller) CloseInstancePorts(ctx context.ProviderCallContext, inst instance.Instance, machineId string, rules []network.IngressRule) error {
	return c.changeIngressRules(ctx, inst, false, rules)
}

// InstanceIngressRules implements Firewaller interface.
func (c *rackspaceFirewaller) InstanceIngressRules(ctx context.ProviderCallContext, inst instance.Instance, machineId string) ([]network.IngressRule, error) {
	_, configurator, err := c.getInstanceConfigurator(ctx, inst)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return configurator.FindIngressRules()
}

func (c *rackspaceFirewaller) changeIngressRules(ctx context.ProviderCallContext, inst instance.Instance, insert bool, rules []network.IngressRule) error {
	addresses, sshClient, err := c.getInstanceConfigurator(ctx, inst)
	if err != nil {
		return errors.Trace(err)
	}

	for _, addr := range addresses {
		if addr.Scope == network.ScopePublic {
			err = sshClient.ChangeIngressRules(addr.Value, insert, rules)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (c *rackspaceFirewaller) getInstanceConfigurator(ctx context.ProviderCallContext, inst instance.Instance) ([]network.Address, common.InstanceConfigurator, error) {
	addresses, err := inst.Addresses(ctx)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if len(addresses) == 0 {
		return addresses, nil, errors.New("No addresses found")
	}

	client := common.NewSshInstanceConfigurator(addresses[0].Value)
	return addresses, client, err
}
