// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/common"
)

type environInstance struct {
	base *mo.VirtualMachine
	env  *environ
}

var _ instances.Instance = (*environInstance)(nil)

func newInstance(base *mo.VirtualMachine, env *environ) *environInstance {
	return &environInstance{
		base: base,
		env:  env,
	}
}

// Id implements instances.Instance.
func (inst *environInstance) Id() instance.Id {
	return instance.Id(inst.base.Name)
}

// Status implements instances.Instance.
func (inst *environInstance) Status(ctx envcontext.ProviderCallContext) instance.Status {
	instanceStatus := instance.Status{
		Status:  status.Empty,
		Message: string(inst.base.Runtime.PowerState),
	}
	switch inst.base.Runtime.PowerState {
	case types.VirtualMachinePowerStatePoweredOn:
		instanceStatus.Status = status.Running
	}
	return instanceStatus
}

// Addresses implements instances.Instance.
func (inst *environInstance) Addresses(ctx envcontext.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	if inst.base.Guest == nil {
		return nil, nil
	}
	res := make([]corenetwork.ProviderAddress, 0, len(inst.base.Guest.Net))
	for _, net := range inst.base.Guest.Net {
		for _, ip := range net.IpAddress {
			res = append(res, corenetwork.NewMachineAddress(ip).AsProviderAddress())
		}
	}
	return res, nil
}

// firewall stuff

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) OpenPorts(ctx envcontext.ProviderCallContext, machineID string, rules firewall.IngressRules) error {
	return inst.changeIngressRules(ctx, true, rules)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(ctx envcontext.ProviderCallContext, machineID string, rules firewall.IngressRules) error {
	return inst.changeIngressRules(ctx, false, rules)
}

// IngressRules returns the set of ports open on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) IngressRules(ctx envcontext.ProviderCallContext, machineID string) (firewall.IngressRules, error) {
	_, client, err := inst.getInstanceConfigurator(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.FindIngressRules()
}

func (inst *environInstance) changeIngressRules(ctx envcontext.ProviderCallContext, insert bool, rules firewall.IngressRules) error {
	if inst.env.ecfg.externalNetwork() == "" {
		// Open/Close port without an externalNetwork defined is treated as a no-op.
		// We don't firewall the internal network, and without an external network we don't have any iptables rules
		// to define.
		logger.Warningf(ctx, "ingress rules changing without an external network defined, no changes will be made")
		return nil
	}
	addresses, client, err := inst.getInstanceConfigurator(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	for _, addr := range addresses {
		if addr.Type == corenetwork.IPv6Address || addr.Scope != corenetwork.ScopePublic {
			// TODO(axw) support firewalling IPv6
			continue
		}
		if err := client.ChangeIngressRules(addr.Value, insert, rules); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (inst *environInstance) getInstanceConfigurator(
	ctx envcontext.ProviderCallContext,
) ([]corenetwork.ProviderAddress, common.InstanceConfigurator, error) {
	addresses, err := inst.Addresses(ctx)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var localAddr string
	for _, addr := range addresses {
		if addr.Scope == corenetwork.ScopeCloudLocal {
			localAddr = addr.Value
			break
		}
	}

	client := common.NewSshInstanceConfigurator(localAddr)
	return addresses, client, err
}
