// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

// LinkLayerDevice is an indirection for state.LinkLayerDevice.
// It facilitates testing the provisioner's use of this package.
type LinkLayerDevice interface {
	Name() string
	Type() state.LinkLayerDeviceType
	MACAddress() string
	ParentName() string
	ParentDevice() (LinkLayerDevice, error)
	EthernetDeviceForBridge(name string) (state.LinkLayerDeviceArgs, error)
	Addresses() ([]*state.Address, error)

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
	return &linkLayerDevice{dev}, nil
}

var _ LinkLayerDevice = (*linkLayerDevice)(nil)

// Machine is an indirection for state.Machine,
// describing a machine that is to host containers.
type Machine interface {
	Id() string
	AllSpaces() (set.Strings, error)
	LinkLayerDevicesForSpaces([]string) (map[string][]LinkLayerDevice, error)
	SetLinkLayerDevices(devicesArgs ...state.LinkLayerDeviceArgs) (err error)
	AllLinkLayerDevices() ([]LinkLayerDevice, error)

	// TODO (manadart 2018-10-10) These methods are used in tests, which rely
	// on the StateSuite. Some of them are recruited via the Container
	// interface below, but they are all located here for simplicity.
	// A better approach could be sought that does not require their
	// presence here.
	SetDevicesAddresses(devicesAddresses ...state.LinkLayerDeviceAddress) (err error)
	SetParentLinkLayerDevicesBeforeTheirChildren(devicesArgs []state.LinkLayerDeviceArgs) error
	SetConstraints(cons constraints.Value) (err error)
	RemoveAllAddresses() error
	Raw() *state.Machine
}

// MachineShim implements Machine.
// It is required to mock the return of LinkLayerDevicesForSpaces,
// which includes raw state.LinkLayerDevice references.
type MachineShim struct {
	*state.Machine
}

// LinkLayerDevicesForSpaces implements Machine by unwrapping the inner
// state.Machine call and wrapping the raw state.LinkLayerDevice references
// with the local LinkLayerDevice implementation.
func (m *MachineShim) LinkLayerDevicesForSpaces(spaces []string) (map[string][]LinkLayerDevice, error) {
	spaceDevs, err := m.Machine.LinkLayerDevicesForSpaces(spaces)
	if err != nil {
		return nil, errors.Trace(err)
	}

	wrapped := make(map[string][]LinkLayerDevice, len(spaceDevs))
	for space, devs := range spaceDevs {
		wrappedDevs := make([]LinkLayerDevice, len(devs))
		for i, d := range devs {
			wrappedDevs[i] = &linkLayerDevice{d}
		}
		wrapped[space] = wrappedDevs
	}
	return wrapped, nil
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
		wrapped[i] = &linkLayerDevice{d}
	}
	return wrapped, nil
}

// Raw returns the inner state.Machine reference.
func (m *MachineShim) Raw() *state.Machine {
	return m.Machine
}

// Machine is an indirection for state.Machine,
// describing a container.
type Container interface {
	Machine
	ContainerType() instance.ContainerType
	DesiredSpaces() (set.Strings, error)
}

var _ Container = (*MachineShim)(nil)
