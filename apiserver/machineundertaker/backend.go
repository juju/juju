// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// Backend defines the methods the machine undertaker needs from
// state.State.
type Backend interface {

	// AllRemovedMachines returns all of the machines which have been
	// marked for removal.
	AllMachineRemovals() ([]string, error)

	// CompleteMachineRemovals removes the machines (and the associated removal
	// requests) after the provider-level cleanup is done.
	CompleteMachineRemovals(machineIDs []string) error

	// WatchMachineRemovals returns a NotifyWatcher that triggers
	// whenever machine removal requests are added or removed.
	WatchMachineRemovals() state.NotifyWatcher

	// Machine gets a specific machine, so we can collect details of
	// its network interfaces.
	Machine(id string) (Machine, error)
}

// Machine defines the methods we need from state.Machine.
type Machine interface {

	// AllLinkLayerDevices returns all of the link-layer devices
	// belonging to this machine.
	AllLinkLayerDevices() ([]LinkLayerDevice, error)
}

// LinkLayerDevice defines the methods we need from
// state.LinkLayerDevice.
type LinkLayerDevice interface {

	// Name returns the name of this device as it appears on the
	// machine.
	Name() string

	// MACAddress returns the media access control (MAC) address of
	// the device.
	MACAddress() string

	// Type returns this device's underlying type.
	Type() state.LinkLayerDeviceType

	// MTU returns the maximum transmission unit the device can handle.
	MTU() uint

	// ProviderID returns the provider-specific device ID, if set.
	ProviderID() network.Id

	// Addresses returns all IP addresses assigned to the device.
	Addresses() ([]Address, error)
}

// Address defines the methods we need from state.Address.
type Address interface {

	// Value returns the actual IP address.
	Value() string

	// ConfigMethod returns the AddressConfigMethod used for this IP
	// address.
	ConfigMethod() state.AddressConfigMethod

	// DNSServers returns the list of DNS nameservers to use, which
	// can be empty.
	DNSServers() []string

	// DNSSearchDomains returns the list of DNS domains to use for
	// qualifying hostnames. Can be empty.
	DNSSearchDomains() []string

	// GatewayAddress returns the gateway address to use, which can be
	// empty.
	GatewayAddress() string

	// ProviderID returns the provider-specific IP address ID, if set.
	ProviderID() network.Id
}

type backendShim struct {
	*state.State
}

func (b *backendShim) Machine(id string) (Machine, error) {
	machine, err := b.State.Machine(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MachineShim{machine}, nil
}

type machineShim struct {
	*state.Machine
}

func (m *machineShim) AllLinkLayerDevices() ([]LinkLayerDevice, error) {
	var result []LinkLayerDevice
	devices, err := m.Machine.AllLinkLayerDevices()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, device := range devices {
		result = append(result, &linkLayerDeviceShim{device})
	}
	return result, nil
}

type linkLayerDeviceShim struct {
	*state.LinkLayerDevice
}

func (d *linkLayerDeviceShim) Addresses() ([]Address, error) {
	var result []Address
	addresses, err := d.LinkLayerDevice.Addresses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, address := range addresses {
		result = append(result, address)
	}
	return result, nil
}
