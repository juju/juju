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

// firewall stuff

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	env := inst.env.getSnapshot()

	// Compose the full set of open ports.
	currentPorts, err := inst.Ports(machineId)
	if err != nil {
		return errors.Trace(err)
	}
	inputPortsSet := network.NewPortSet(ports...)
	if inputPortsSet.IsEmpty() {
		return nil
	}
	currentPortsSet := network.NewPortSet(currentPorts...)

	// Send the request, depending on the current ports.
	if currentPortsSet.IsEmpty() {
		firewall := firewallSpec(machineId, inputPortsSet)
		if err := env.gce.setFirewall("", firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}

	} else {
		newPortsSet := currentPortsSet.Union(inputPortsSet)
		firewall := firewallSpec(machineId, newPortsSet)
		if err := env.gce.setFirewall(machineId, firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}
	}
	return nil
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	env := inst.env.getSnapshot()

	// Compose the full set of open ports.
	currentPorts, err := inst.Ports(machineId)
	if err != nil {
		return errors.Trace(err)
	}
	inputPortsSet := network.NewPortSet(ports...)
	if inputPortsSet.IsEmpty() {
		return nil
	}
	currentPortsSet := network.NewPortSet(currentPorts...)
	newPortsSet := currentPortsSet.Difference(inputPortsSet)

	// Send the request, depending on the current ports.
	if newPortsSet.IsEmpty() {
		if err := env.gce.setFirewall(machineId, nil); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	} else {
		firewall := firewallSpec(machineId, newPortsSet)
		if err := env.gce.setFirewall(machineId, firewall); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	}
	return nil
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by SortPorts.
func (inst *environInstance) Ports(machineId string) ([]network.PortRange, error) {
	env := inst.env.getSnapshot()

	firewall, err := env.gce.firewall(machineId)
	if err != nil {
		return nil, errors.Annotate(err, "while getting ports from GCE")
	}

	var ports []network.PortRange
	for _, allowed := range firewall.Allowed {
		for _, portRangeStr := range allowed.Ports {
			portRange, err := network.ParsePortRangePorts(portRangeStr, allowed.IPProtocol)
			if err != nil {
				return ports, errors.Annotate(err, "bad ports from GCE")
			}
			ports = append(ports, portRange)
		}
	}

	return ports, nil
}
