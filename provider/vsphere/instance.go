// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/status"
)

type environInstance struct {
	base *mo.VirtualMachine
	env  *environ
}

var _ instance.Instance = (*environInstance)(nil)

func newInstance(base *mo.VirtualMachine, env *environ) *environInstance {
	return &environInstance{
		base: base,
		env:  env,
	}
}

// Id implements instance.Instance.
func (inst *environInstance) Id() instance.Id {
	return instance.Id(inst.base.Name)
}

// Status implements instance.Instance.
func (inst *environInstance) Status() instance.InstanceStatus {
	instanceStatus := instance.InstanceStatus{
		Status:  status.Empty,
		Message: string(inst.base.Runtime.PowerState),
	}
	switch inst.base.Runtime.PowerState {
	case types.VirtualMachinePowerStatePoweredOn:
		instanceStatus.Status = status.Running
	}
	return instanceStatus
}

// Addresses implements instance.Instance.
func (inst *environInstance) Addresses() ([]network.Address, error) {
	if inst.base.Guest == nil {
		return nil, nil
	}
	res := make([]network.Address, 0, len(inst.base.Guest.Net))
	for _, net := range inst.base.Guest.Net {
		for _, ip := range net.IpAddress {
			res = append(res, network.NewAddress(ip))
		}
	}
	return res, nil
}

// firewall stuff

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) OpenPorts(machineID string, rules []network.IngressRule) error {
	return inst.changeIngressRules(true, rules)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineID string, rules []network.IngressRule) error {
	return inst.changeIngressRules(false, rules)
}

// IngressRules returns the set of ports open on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) IngressRules(machineID string) ([]network.IngressRule, error) {
	_, client, err := inst.getInstanceConfigurator()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.FindIngressRules()
}

func (inst *environInstance) changeIngressRules(insert bool, rules []network.IngressRule) error {
	if inst.env.ecfg.externalNetwork() == "" {
		return errors.New("Can't close/open ports without external network")
	}
	addresses, client, err := inst.getInstanceConfigurator()
	if err != nil {
		return errors.Trace(err)
	}

	for _, addr := range addresses {
		if addr.Scope == network.ScopePublic {
			err = client.ChangeIngressRules(addr.Value, insert, rules)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (inst *environInstance) getInstanceConfigurator() ([]network.Address, common.InstanceConfigurator, error) {
	addresses, err := inst.Addresses()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var localAddr string
	for _, addr := range addresses {
		if addr.Scope == network.ScopeCloudLocal {
			localAddr = addr.Value
			break
		}
	}

	client := common.NewSshInstanceConfigurator(localAddr)
	return addresses, client, err
}
