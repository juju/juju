// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"
	"sync"

	"gopkg.in/amz.v2/ec2"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type ec2Instance struct {
	e *environ

	mu sync.Mutex
	*ec2.Instance
}

func (inst *ec2Instance) String() string {
	return string(inst.Id())
}

var _ instance.Instance = (*ec2Instance)(nil)

func (inst *ec2Instance) getInstance() *ec2.Instance {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.Instance
}

func (inst *ec2Instance) Id() instance.Id {
	return instance.Id(inst.getInstance().InstanceId)
}

func (inst *ec2Instance) Status() string {
	return inst.getInstance().State.Name
}

// Refresh implements instance.Refresh(), requerying the
// Instance details over the ec2 api
func (inst *ec2Instance) Refresh() error {
	_, err := inst.refresh()
	return err
}

// refresh requeries Instance details over the ec2 api.
func (inst *ec2Instance) refresh() (*ec2.Instance, error) {
	id := inst.Id()
	insts, err := inst.e.Instances([]instance.Id{id})
	if err != nil {
		return nil, err
	}
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.Instance = insts[0].(*ec2Instance).Instance
	return inst.Instance, nil
}

// Addresses implements network.Addresses() returning generic address
// details for the instance, and requerying the ec2 api if required.
func (inst *ec2Instance) Addresses() ([]network.Address, error) {
	// TODO(gz): Stop relying on this requerying logic, maybe remove error
	instInstance := inst.getInstance()
	var addresses []network.Address
	possibleAddresses := []network.Address{
		{
			Value: instInstance.IPAddress,
			Type:  network.IPv4Address,
			Scope: network.ScopePublic,
		},
		{
			Value: instInstance.PrivateIPAddress,
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
	}
	for _, address := range possibleAddresses {
		if address.Value != "" {
			addresses = append(addresses, address)
		}
	}
	return addresses, nil
}

func (inst *ec2Instance) OpenPorts(machineId string, ports []network.PortRange) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.openPortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("opened ports in security group %s: %v", name, ports)
	return nil
}

func (inst *ec2Instance) ClosePorts(machineId string, ports []network.PortRange) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.closePortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("closed ports in security group %s: %v", name, ports)
	return nil
}

func (inst *ec2Instance) Ports(machineId string) ([]network.PortRange, error) {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	ranges, err := inst.e.portsInGroup(name)
	if err != nil {
		return nil, err
	}
	return ranges, nil
}
