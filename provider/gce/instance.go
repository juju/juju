// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type environInstance struct {
	id   instance.Id
	env  *environ
	zone string

	gce *compute.Instance
}

var _ instance.Instance = (*environInstance)(nil)

func newInstance(raw *compute.Instance, env *environ) *environInstance {
	return &environInstance{
		id:   instance.Id(raw.Name),
		env:  env,
		zone: raw.Zone,
		gce:  raw,
	}
}

func (inst *environInstance) getInstance() *compute.Instance {
	return inst.gce
}

func (inst *environInstance) Id() instance.Id {
	return inst.id
}

func (inst *environInstance) Status() string {
	return inst.gce.Status
}

func (inst *environInstance) update(env *environ, newInst *compute.Instance) {
	inst.gce = newInst
}

func (inst *environInstance) Refresh() error {
	env := inst.env.getSnapshot()

	// TODO(ericsnow) is zone the right thing
	gInst, err := env.gce.instance(inst.zone, string(inst.id))
	if err != nil {
		return errors.Trace(err)
	}

	inst.update(env, gInst)
	return nil
}

func (inst *environInstance) Addresses() ([]network.Address, error) {
	var addresses []network.Address

	for _, netif := range inst.gce.NetworkInterfaces {
		// Add public addresses.
		for _, accessConfig := range netif.AccessConfigs {
			if accessConfig.NatIP == "" {
				continue
			}
			address := network.Address{
				Value: accessConfig.NatIP,
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			}
			addresses = append(addresses, address)

		}

		// Add private address.
		// TODO(ericsnow) Are these really the internal addresses?
		if netif.NetworkIP == "" {
			continue
		}
		address := network.Address{
			Value: netif.NetworkIP,
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
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
func (inst *environInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	env := inst.env.getSnapshot()
	err := env.openPorts(machineId, ports)
	return errors.Trace(err)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	env := inst.env.getSnapshot()
	err := env.closePorts(machineId, ports)
	return errors.Trace(err)
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by SortPorts.
func (inst *environInstance) Ports(machineId string) ([]network.PortRange, error) {
	env := inst.env.getSnapshot()
	ports, err := env.ports(machineId)
	return ports, errors.Trace(err)
}
