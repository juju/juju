// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

func (gce *Connection) firewall(name string) (*compute.Firewall, error) {
	call := gce.raw.Firewalls.List(gce.ProjectID)
	call = call.Filter("name eq " + name)
	firewallList, err := call.Do()
	if err != nil {
		return nil, errors.Annotate(err, "while getting firewall from GCE")
	}
	if len(firewallList.Items) == 0 {
		return nil, errors.NotFoundf("firewall %q", name)
	}
	return firewallList.Items[0], nil
}

func (gce *Connection) insertFirewall(firewall *compute.Firewall) error {
	call := gce.raw.Firewalls.Insert(gce.ProjectID, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (gce *Connection) updateFirewall(name string, firewall *compute.Firewall) error {
	call := gce.raw.Firewalls.Update(gce.ProjectID, name, firewall)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (gce *Connection) deleteFirewall(name string) error {
	call := gce.raw.Firewalls.Delete(gce.ProjectID, name)
	operation, err := call.Do()
	if err != nil {
		return errors.Trace(err)
	}
	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (gce Connection) Ports(fwname string) ([]network.PortRange, error) {
	firewall, err := gce.firewall(fwname)
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "while getting ports from GCE")
	}

	var ports []network.PortRange
	for _, allowed := range firewall.Allowed {
		for _, portRangeStr := range allowed.Ports {
			portRange, err := network.ParsePortRange(portRangeStr)
			if err != nil {
				return ports, errors.Annotate(err, "bad ports from GCE")
			}
			portRange.Protocol = allowed.IPProtocol
			ports = append(ports, *portRange)
		}
	}

	return ports, nil
}

func (gce Connection) OpenPorts(name string, ports []network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := gce.Ports(name)
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
		firewall := firewallSpec(name, inputPortsSet)
		if err := gce.insertFirewall(firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}

	} else {
		newPortsSet := currentPortsSet.Union(inputPortsSet)
		firewall := firewallSpec(name, newPortsSet)
		if err := gce.updateFirewall(name, firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}
	}
	return nil
}

func (gce Connection) ClosePorts(name string, ports []network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := gce.Ports(name)
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
		if err := gce.deleteFirewall(name); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	} else {
		firewall := firewallSpec(name, newPortsSet)
		if err := gce.updateFirewall(name, firewall); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	}
	return nil
}
