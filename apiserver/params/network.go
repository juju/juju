// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/juju/network"
)

// PortsResults holds the bulk operation result of an API call
// that returns a slice of Port.
type PortsResults struct {
	Results []PortsResult `json:"Results"`
}

// PortsResult holds the result of an API call that returns a slice
// of Port or an error.
type PortsResult struct {
	Error *Error `json:"Error"`
	Ports []Port `json:"Ports"`
}

// Port encapsulates the protocol and the number of a port.
type Port struct {
	Protocol string `json:"Protocol"`
	Number   int    `json:"Number"`
}

// Address represents the location of a machine, including metadata
// about what kind of location the address describes.
type Address struct {
	Value       string `json:"Value"`
	Type        string `json:"Type"`
	NetworkName string `json:"NetworkName"`
	Scope       string `json:"Scope"`
}

// FromNetworkAddress is a convenience helper to create a parameter
// out of the network type.
func FromNetworkAddress(addr network.Address) Address {
	return Address{
		Value:       addr.Value,
		Type:        string(addr.Type),
		NetworkName: addr.NetworkName,
		Scope:       string(addr.Scope),
	}
}

// NetworkAddress is a convenience helper to return the parameter
// as network type.
func (addr Address) NetworkAddress() network.Address {
	return network.Address{
		Value:       addr.Value,
		Type:        network.AddressType(addr.Type),
		NetworkName: addr.NetworkName,
		Scope:       network.Scope(addr.Scope),
	}
}

// HostPort combines the network address information of a machine including its
// location, metadata about what kind of location, and a port.
type HostPort struct {
	Address
	Port int `json:"Port"`
}

// FromNetworkHostPort is a convenience helper to create a parameter
// out of the network type.
func FromNetworkHostPort(hp network.HostPort) HostPort {
	return HostPort{FromNetworkAddress(hp.Address), hp.Port}
}

// NetworkHostPort is a convenience helper to return the parameter
// as network type.
func (hp HostPort) NetworkHostPort() network.HostPort {
	return network.HostPort{hp.Address.NetworkAddress(), hp.Port}
}

// MachineAddresses holds an machine tag and addresses.
type MachineAddresses struct {
	Tag       string    `json:"Tag"`
	Addresses []Address `json:"Addresses"`
}
