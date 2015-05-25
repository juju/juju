// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/govmomi/vim25/mo"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
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
func (inst *environInstance) Status() string {
	//return inst.base.Status()
	return ""
}

// Refresh implements instance.Instance.
func (inst *environInstance) Refresh() error {
	env := inst.env.getSnapshot()
	err := env.client.Refresh(inst.base)
	return errors.Trace(err)
}

// Addresses implements instance.Instance.
func (inst *environInstance) Addresses() ([]network.Address, error) {
	if inst.base.Guest == nil || inst.base.Guest.IpAddress == "" {
		return nil, nil
	}
	res := make([]network.Address, 0)
	for _, net := range inst.base.Guest.Net {
		for _, ip := range net.IpAddress {
			scope := network.ScopeCloudLocal
			if net.Network == inst.env.ecfg.externalNetwork() {
				scope = network.ScopePublic
			}
			res = append(res, network.NewScopedAddress(ip, scope))
		}
	}
	return res, nil
}

func findInst(id instance.Id, instances []instance.Instance) instance.Instance {
	for _, inst := range instances {
		if id == inst.Id() {
			return inst
		}
	}
	return nil
}

// firewall stuff

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) OpenPorts(machineID string, ports []network.PortRange) error {
	return inst.changePorts(true, ports)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineID string, ports []network.PortRange) error {
	return inst.changePorts(false, ports)
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) Ports(machineID string) ([]network.PortRange, error) {
	_, sshClient, err := inst.getSshClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sshClient.findOpenPorts()
}

func (inst *environInstance) changePorts(insert bool, ports []network.PortRange) error {
	if inst.env.ecfg.externalNetwork() == "" {
		return errors.New("Can't close/open ports without external network")
	}
	addresses, sshClient, err := inst.getSshClient()
	if err != nil {
		return errors.Trace(err)
	}

	for _, addr := range addresses {
		if addr.Scope == network.ScopePublic {
			err = sshClient.changePorts(addr.Value, insert, ports)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (inst *environInstance) getSshClient() ([]network.Address, *sshClient, error) {
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

	client := newSshClient(localAddr)
	return addresses, client, err
}
