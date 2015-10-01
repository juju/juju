// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

// Ports build a list of all open port ranges for a given firewall name
// (within the Connection's project) and returns it. If the firewall
// does not exist then the list will be empty and no error is returned.
func (client Client) Ports(fwname string) ([]network.PortRange, error) {
	return nil, errors.NotImplementedf("")
}

// OpenPorts sends a request to the API to open the provided port
// ranges on the named firewall. If the firewall does not exist yet it
// is created, with the provided port ranges opened. Otherwise the
// existing firewall is updated to add the provided port ranges to the
// ports it already has open. The call blocks until the ports are
// opened or the request fails.
func (client Client) OpenPorts(fwname string, ports ...network.PortRange) error {
	// TODO(ericsnow) Short-circuit if ports is empty.

	// Compose the full set of open ports.
	currentPorts, err := client.Ports(fwname)
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
		// Create a new firewall.
		firewall := firewallSpec(fwname, inputPortsSet)
		return errors.NotImplementedf("")
		firewall = firewall
		return nil
	}

	// Update an existing firewall.
	newPortsSet := currentPortsSet.Union(inputPortsSet)
	firewall := firewallSpec(fwname, newPortsSet)
	return errors.NotImplementedf("")
	firewall = firewall
	return nil
}

// ClosePorts sends a request to the API to close the provided port
// ranges on the named firewall. If the firewall does not exist nothing
// happens. If the firewall is left with no ports then it is removed.
// Otherwise it will be left with just the open ports it has that do not
// match the provided port ranges. The call blocks until the ports are
// closed or the request fails.
func (client Client) ClosePorts(fwname string, ports ...network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := client.Ports(fwname)
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
		// Delete a firewall.
		return errors.NotImplementedf("")
		// TODO(ericsnow) Also, handle case where firewall does not exist.
	}

	// Update an existing firewall.
	firewall := firewallSpec(fwname, newPortsSet)
	return errors.NotImplementedf("")
	firewall = firewall
	return nil
}
