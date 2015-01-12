// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

// firewall sends an API request to GCE for the information about
// the named firewall and returns it. If the firewall is not found,
// errors.NotFound is returned.
func (gce *Connection) firewall(name string) (*compute.Firewall, error) {
	call := gce.raw.Firewalls.List(gce.ProjectID)
	call = call.Filter("name eq " + name)

	svc := Services{FirewallList: call}
	result, err := doCall(svc)
	if err != nil {
		return nil, errors.Annotate(err, "while getting firewall from GCE")
	}
	firewallList, ok := result.(compute.FirewallList)
	if !ok {
		return nil, errors.New("unable to convert result to compute.FirewallList")
	}

	if len(firewallList.Items) == 0 {
		return nil, errors.NotFoundf("firewall %q", name)
	}
	return firewallList.Items[0], nil
}

// insertFirewall requests GCE to add a firewall with the provided info.
// If the firewall already exists then an error will be returned.
// The call blocks until the firewall is added or the request fails.
func (gce *Connection) insertFirewall(firewall *compute.Firewall) error {
	svc := Services{
		FirewallInsert: gce.raw.Firewalls.Insert(gce.ProjectID, firewall),
	}
	result, err := doCall(svc)
	if err != nil {
		return errors.Trace(err)
	}

	operation, ok := result.(*compute.Operation)
	if !ok {
		return errors.New("unable to convert result to compute.Operation")
	}

	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// updateFirewall requests GCE to update the named firewall with the
// provided info, overwriting the existing data. If the firewall does
// not exist then an error will be returned. The call blocks until the
// firewall is updated or the request fails.
func (gce *Connection) updateFirewall(name string, firewall *compute.Firewall) error {
	svc := Services{
		FirewallUpdate: gce.raw.Firewalls.Update(gce.ProjectID, name, firewall),
	}
	result, err := doCall(svc)
	if err != nil {
		return errors.Trace(err)
	}

	operation, ok := result.(*compute.Operation)
	if !ok {
		return errors.New("unable to convert result to compute.Operation")
	}

	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// deleteFirewall removed the named firewall from the conenction's
// project. If it does not exist then this is a noop. The call blocks
// until the firewall is added or the request fails.
func (gce *Connection) deleteFirewall(name string) error {
	svc := Services{
		FirewallDelete: gce.raw.Firewalls.Delete(gce.ProjectID, name),
	}

	result, err := doCall(svc)
	if err != nil {
		return errors.Trace(err)
	}

	operation, ok := result.(*compute.Operation)
	if !ok {
		return errors.New("unable to covert result to compute.Operation")
	}

	if err := gce.waitOperation(operation, attemptsLong); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Ports build a list of all open port ranges for a given firewall name
// (within the Connection's project) and returns it. If the firewall
// does not exist then the list will be empty and no error is returned.
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

// OpenPorts sends a request to the GCE API to open the provided port
// ranges on the named firewall. If the firewall does not exist yet it
// is created, with the provided port ranges opened. Otherwise the
// existing firewall is updated to add the provided port ranges to the
// ports it already has open. The call blocks until the ports are
// opened or the request fails.
func (gce Connection) OpenPorts(fwname string, ports []network.PortRange) error {
	// TODO(ericsnow) Short-circuit if ports is empty.

	// Compose the full set of open ports.
	currentPorts, err := gce.Ports(fwname)
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
		firewall := firewallSpec(fwname, inputPortsSet)
		if err := gce.insertFirewall(firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}

	} else {
		newPortsSet := currentPortsSet.Union(inputPortsSet)
		firewall := firewallSpec(fwname, newPortsSet)
		if err := gce.updateFirewall(fwname, firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}
	}
	return nil
}

// ClosePorts sends a request to the GCE API to close the provided port
// ranges on the named firewall. If the firewall does not exist nothing
// happens. If the firewall is left with no ports then it is removed.
// Otherwise it will be left with just the open ports it has that do not
// match the provided port ranges. The call blocks until the ports are
// closed or the request fails.
func (gce Connection) ClosePorts(fwname string, ports []network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := gce.Ports(fwname)
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
		// TODO(ericsnow) Handle case where firewall does not exist.
		if err := gce.deleteFirewall(fwname); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	} else {
		firewall := firewallSpec(fwname, newPortsSet)
		if err := gce.updateFirewall(fwname, firewall); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	}
	return nil
}
