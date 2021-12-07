// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/openstack"
)

type firewallerFactory struct {
}

var _ openstack.FirewallerFactory = (*firewallerFactory)(nil)

// GetFirewaller implements FirewallerFactory
func (f *firewallerFactory) GetFirewaller(env environs.Environ) openstack.Firewaller {
	return &equinixFirewaller{}
}

type equinixFirewaller struct{}

var _ openstack.Firewaller = (*equinixFirewaller)(nil)

// OpenPorts is not supported.
func (c *equinixFirewaller) OpenPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	return errors.NotSupportedf("OpenPorts")
}

// ClosePorts is not supported.
func (c *equinixFirewaller) ClosePorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	return errors.NotSupportedf("ClosePorts")
}

// IngressRules returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (c *equinixFirewaller) IngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error) {
	return nil, errors.NotSupportedf("Ports")
}

// DeleteGroups implements OpenstackFirewaller interface.
func (c *equinixFirewaller) DeleteGroups(ctx context.ProviderCallContext, names ...string) error {
	return nil
}

// DeleteAllModelGroups implements OpenstackFirewaller interface.
func (c *equinixFirewaller) DeleteAllModelGroups(ctx context.ProviderCallContext) error {
	return nil
}

// DeleteAllControllerGroups implements OpenstackFirewaller interface.
func (c *equinixFirewaller) DeleteAllControllerGroups(ctx context.ProviderCallContext, controllerUUID string) error {
	return nil
}

func (c *equinixFirewaller) UpdateGroupController(ctx context.ProviderCallContext, controllerUUID string) error {
	return nil
}

// GetSecurityGroups implements OpenstackFirewaller interface.
func (c *equinixFirewaller) GetSecurityGroups(ctx context.ProviderCallContext, ids ...instance.Id) ([]string, error) {
	return nil, nil
}

// SetUpGroups implements OpenstackFirewaller interface.
func (c *equinixFirewaller) SetUpGroups(ctx context.ProviderCallContext, controllerUUID, machineId string, apiPort int) ([]string, error) {
	return nil, nil
}

// OpenInstancePorts implements Firewaller interface.
func (c *equinixFirewaller) OpenInstancePorts(ctx context.ProviderCallContext, inst instances.Instance, machineId string, rules firewall.IngressRules) error {
	return c.changeIngressRules(ctx, inst, true, rules)
}

// CloseInstancePorts implements Firewaller interface.
func (c *equinixFirewaller) CloseInstancePorts(ctx context.ProviderCallContext, inst instances.Instance, machineId string, rules firewall.IngressRules) error {
	return c.changeIngressRules(ctx, inst, false, rules)
}

// InstanceIngressRules implements Firewaller interface.
func (c *equinixFirewaller) InstanceIngressRules(ctx context.ProviderCallContext, inst instances.Instance, machineId string) (firewall.IngressRules, error) {
	_, configurator, err := c.getInstanceConfigurator(ctx, inst)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rules, err := configurator.FindIngressRules()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
	}
	return rules, err
}

func (c *equinixFirewaller) changeIngressRules(ctx context.ProviderCallContext, inst instances.Instance, insert bool, rules firewall.IngressRules) error {
	addresses, sshClient, err := c.getInstanceConfigurator(ctx, inst)
	if err != nil {
		return errors.Trace(err)
	}

	for _, addr := range addresses {
		if addr.Scope == network.ScopePublic {
			err = sshClient.ChangeIngressRules(addr.Value, insert, rules)
			if err != nil {
				common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (c *equinixFirewaller) getInstanceConfigurator(
	ctx context.ProviderCallContext, inst instances.Instance,
) ([]network.ProviderAddress, common.InstanceConfigurator, error) {
	addresses, err := inst.Addresses(ctx)
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nil, nil, errors.Trace(err)
	}
	if len(addresses) == 0 {
		return addresses, nil, errors.New("No addresses found")
	}

	client := common.NewSshInstanceConfigurator(addresses[0].Value)
	return addresses, client, err
}
