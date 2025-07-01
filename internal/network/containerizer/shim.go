// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

// LinkLayerDevice is an indirection for state.LinkLayerDevice.
// It facilitates testing the provisioner's use of this package.
type LinkLayerDevice interface {
	Name() string
	Type() network.LinkLayerDeviceType
	MACAddress() string
	ParentName() string
	ParentDevice() (LinkLayerDevice, error)
	EthernetDeviceForBridge(name string, askForProviderAddress bool, allSubnets network.SubnetInfos) (network.InterfaceInfo, error)
	Addresses() ([]*state.Address, error)
	VirtualPortType() network.VirtualPortType

	// These are recruited in tests. See comment on Machine below.
	MTU() uint
	IsUp() bool
	IsAutoStart() bool
}

// linkLayerDevice implements LinkLayerDevice.
// We need our own implementation of the indirection above in order to mock the
// return of ParentDevice, which in the state package returns a reference to a
// raw state.LinkLayerDevice.
type linkLayerDevice struct {
	*state.LinkLayerDevice
}

// ParentDevice implements LinkLayerDevice by wrapping the response from the
// inner device's call in a new instance of linkLayerDevice.
func (l *linkLayerDevice) ParentDevice() (LinkLayerDevice, error) {
	dev, err := l.LinkLayerDevice.ParentDevice()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewLinkLayerDevice(dev), nil
}

// NewLinkLayerDevice wraps the given state.LinkLayerDevice
// in a linkLayerDevice.
func NewLinkLayerDevice(dev *state.LinkLayerDevice) LinkLayerDevice {
	return &linkLayerDevice{dev}
}

var _ LinkLayerDevice = (*linkLayerDevice)(nil)

// Machine is an indirection for state.Machine,
// describing a machine that is to host containers.
type Machine interface {
	Id() string
	AllDeviceAddresses() ([]Address, error)
	AllSpaces(allSubnets network.SubnetInfos) (set.Strings, error)
	AllLinkLayerDevices() ([]LinkLayerDevice, error)
}

// MachineShim implements Machine.
// It is required to mock the return of LinkLayerDevicesForSpaces,
// which includes raw state.LinkLayerDevice references.
type MachineShim struct {
	*state.Machine
}

// NewMachine wraps the given state.machine in a MachineShim.
func NewMachine(m *state.Machine) *MachineShim {
	return &MachineShim{Machine: m}
}

// AllLinkLayerDevices implements Machine by wrapping each
// state.LinkLayerDevice reference in returned collection with the local
// LinkLayerDevice implementation.
func (m *MachineShim) AllLinkLayerDevices() ([]LinkLayerDevice, error) {
	devs, err := m.Machine.AllLinkLayerDevices()
	if err != nil {
		return nil, errors.Trace(err)
	}

	wrapped := make([]LinkLayerDevice, len(devs))
	for i, d := range devs {
		wrapped[i] = NewLinkLayerDevice(d)
	}
	return wrapped, nil
}

// AllDeviceAddresses implements Machine by wrapping each state.Address
// reference in returned collection with the local Address implementation.
func (m *MachineShim) AllDeviceAddresses() ([]Address, error) {
	addrs, err := m.Machine.AllDeviceAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	wrapped := make([]Address, len(addrs))
	for i, a := range addrs {
		wrapped[i] = NewAddress(a)
	}
	return wrapped, nil
}

// Raw returns the inner state.Machine reference.
func (m *MachineShim) Raw() *state.Machine {
	return m.Machine
}

// Address is an indirection for state.Address.
type Address interface {
	DeviceName() string
	// SubnetCIDR returns the CIDR of the subnet this IP address comes from.
	SubnetCIDR() string
}

// addressShim implements Address.
type addressShim struct {
	*state.Address
}

// NewAddress wraps the given state.Address in an addressShim.
func NewAddress(a *state.Address) Address {
	return &addressShim{Address: a}
}

// Subnet is an indirection for state.Subnet.
type Subnet interface {
	SpaceID() string
}

// Container is an indirection for state.Machine, describing a container.
type Container interface {
	Machine
	ContainerType() instance.ContainerType
	Constraints() (constraints.Value, error)
}

var _ Container = (*MachineShim)(nil)
