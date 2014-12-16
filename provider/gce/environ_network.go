// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

// AllocateAddress requests a specific address to be allocated for the
// given instance on the given network.
func (env *environ) AllocateAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errors.Trace(errNotImplemented)
}

func (env *environ) ReleaseAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errors.Trace(errNotImplemented)
}

func (env *environ) Subnets(inst instance.Id) ([]network.BasicInfo, error) {
	return nil, errors.Trace(errNotImplemented)
}

func (env *environ) ListNetworks(inst instance.Id) ([]network.BasicInfo, error) {
	return nil, errors.Trace(errNotImplemented)
}

func (env *environ) globalFirewallName() string {
	fwName := common.MachineFullName(env, "")
	return fwName[:len(fwName)-1]
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	err := env.openPorts(env.globalFirewallName(), ports)
	return errors.Trace(err)
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	err := env.closePorts(env.globalFirewallName(), ports)
	return errors.Trace(err)
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	ports, err := env.ports(env.globalFirewallName())
	return ports, errors.Trace(err)
}

func (env *environ) openPorts(name string, ports []network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := env.ports(name)
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
		if err := env.gce.setFirewall("", firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}

	} else {
		newPortsSet := currentPortsSet.Union(inputPortsSet)
		firewall := firewallSpec(name, newPortsSet)
		if err := env.gce.setFirewall(name, firewall); err != nil {
			return errors.Annotatef(err, "opening port(s) %+v", ports)
		}
	}
	return nil
}

func (env *environ) closePorts(name string, ports []network.PortRange) error {
	// Compose the full set of open ports.
	currentPorts, err := env.ports(name)
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
		if err := env.gce.setFirewall(name, nil); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	} else {
		firewall := firewallSpec(name, newPortsSet)
		if err := env.gce.setFirewall(name, firewall); err != nil {
			return errors.Annotatef(err, "closing port(s) %+v", ports)
		}
	}
	return nil
}

func (env *environ) ports(name string) ([]network.PortRange, error) {
	firewall, err := env.gce.firewall(name)
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
