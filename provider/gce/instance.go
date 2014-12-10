// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"code.google.com/p/google-api-go-client/compute/v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type environInstance struct {
	id        instance.Id
	env       *environ
	projectID string

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

func (inst *environInstance) Refresh() error {
	env := inst.env.getSnapshot()

	// TODO(ericsnow) is zone the right thing
	call := env.gce.Instances.Get(env.projectID, inst.zone, inst.id)
	gInst, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	inst.gce = gInst
	inst.projectID = env.projectID
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
				Scope: netowkr.ScopePublic,
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
			Scope: netowkr.ScopeCloudLocal,
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}

// firewall stuff

func (inst *environInstance) waitOperation(operation *compute.Operation) error {
	return errNotImplemented
}

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	// Define the firewall.
	// TODO(ericsnow) Should we be re-using the firewall (update vs. insert)?
	firewall := compute.Firewall{
		// Allowed is set below.
		// Description is not set.
		Name: machineId,
		// TODO(ericsnow) Does Network need to be set?
		Network: "",
		// SourceRanges is not set.
		// SourceTags is not set.
		// TargetTags is not set.
	}
	for _, portRange := range ports {
		allowed := compute.FirewallAllowed{
			IPProtocol: portRange.Protocol,
			Ports:      []string{portRange.PortsString()},
		}
		firewall.Allowed = append(inst.firewall.Allowed, allowed)
	}

	// Send the request.
	call := inst.gce.Firewalls.Insert(inst.projectID, &firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Annotatef(err, "opening port %v", port)
	}
	if err := inst.waitOperation(operation); err != nil {
		return errors.Annotatef(err, "opening port %v", port)
	}
	return nil
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	return errNotImplemented
}

// Ports returns the set of ports open on the instance, which
// should have been started with the given machine id.
// The ports are returned as sorted by SortPorts.
func (inst *environInstance) Ports(machineId string) ([]network.PortRange, error) {
	call := inst.gce.Firewalls.Get(inst.projectID, machineId)
	firewall, err := call.Do()
	if err != nil {
		return nil, errors.Annotate(err, "while getting ports from GCE")
	}

	var ports []network.PortRange
	for _, allowed := range firewall.Allowed {
		for _, portRangeStr := range allowed.Ports {
			portRange, err := network.ParsePortRange(portRangeStr, allowed.IPProtocol)
			if err != nil {
				return ports, errors.Annotate(err, "bad ports from GCE")
			}
			ports = append(ports, portRange)
		}
	}

	return ports, nil
}
