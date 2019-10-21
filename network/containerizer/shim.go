// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujucharm "gopkg.in/juju/charm.v6"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

//go:generate mockgen -package containerizer -destination bridgepolicy_mock_test.go github.com/juju/juju/network/containerizer Container,Unit,Application,Spaces,Address,Subnet,LinkLayerDevice,Bindings

// SpaceBacking describes the retrieval of all spaces from the DB.
type SpaceBacking interface {
	AllSpaces() ([]*state.Space, error)
}

// Spaces describes a cache of all space info for a model.
type Spaces interface {
	// GetByID returns the space for the input ID or an error if not found.
	GetByID(id string) (network.SpaceInfo, error)
	// GetByName returns the space for the input name or an error if not found.
	GetByName(name string) (network.SpaceInfo, error)
}

// spaceCache implements Spaces.
type spaceCache struct {
	spaces network.SpaceInfos
}

// NewSpaces uses the input backing to populate and return a cache of spaces.
func NewSpaces(st SpaceBacking) (Spaces, error) {
	spaces, err := st.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}

	infos := make(network.SpaceInfos, len(spaces))
	for i, space := range spaces {
		infos[i] = space.NetworkSpace()
	}
	return &spaceCache{spaces: infos}, nil
}

// GetByID implements Spaces.
func (s *spaceCache) GetByID(id string) (network.SpaceInfo, error) {
	sp := s.spaces.GetByID(id)
	if sp == nil {
		return network.SpaceInfo{}, errors.NotFoundf("space with ID %q", id)
	}
	return *sp, nil
}

// GetByName implements Spaces.
func (s *spaceCache) GetByName(name string) (network.SpaceInfo, error) {
	sp := s.spaces.GetByName(name)
	if sp == nil {
		return network.SpaceInfo{}, errors.NotFoundf("space with name %q", name)
	}
	return *sp, nil
}

// LinkLayerDevice is an indirection for state.LinkLayerDevice.
// It facilitates testing the provisioner's use of this package.
type LinkLayerDevice interface {
	Name() string
	Type() network.LinkLayerDeviceType
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
	AllAddresses() ([]Address, error)
	AllSpaces() (set.Strings, error)
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

func NewMachine(m *state.Machine) *MachineShim {
	return &MachineShim{m}
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

func (m *MachineShim) AllAddresses() ([]Address, error) {
	addrs, err := m.Machine.AllAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	wrapped := make([]Address, len(addrs))
	for i, a := range addrs {
		wrapped[i] = &addressShim{a}
	}
	return wrapped, nil
}

// Raw returns the inner state.Machine reference.
func (m *MachineShim) Raw() *state.Machine {
	return m.Machine
}

// Address is an indirection for state.Address
type Address interface {
	Subnet() (Subnet, error)
	DeviceName() string
}

type addressShim struct {
	*state.Address
}

func (a *addressShim) Subnet() (Subnet, error) {
	return a.Address.Subnet()
}

// Subnet is an indirection for state.Subnet
type Subnet interface {
	SpaceID() string
}

// Container is an indirection for state.Machine,
// describing a container.
type Container interface {
	Machine
	ContainerType() instance.ContainerType
	Units() ([]Unit, error)
	Constraints() (constraints.Value, error)
}

var _ Container = (*MachineShim)(nil)

func (m *MachineShim) Units() ([]Unit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}
	wrapped := make([]Unit, len(units))
	for i, u := range units {
		wrapped[i] = &unitShim{u}
	}
	return wrapped, nil
}

// Unit, Application & Charm are used to facilitate mocks
// for testing in apiserver/.../agent/provisioner.  This is a
// by product of bad design.

// unitShim implements Unit.
// It is required to mock the return of Units from MachineShim.
type unitShim struct {
	*state.Unit
}

var _ Unit = (*unitShim)(nil)

type Unit interface {
	Application() (Application, error)
	Name() string
}

func (u *unitShim) Application() (Application, error) {
	app, err := u.Unit.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &applicationShim{app}, nil
}

// applicationShim implements Application.
// It is required to mock the return an Application from unitShim.
type applicationShim struct {
	*state.Application
}

var _ Application = (*applicationShim)(nil)

type Application interface {
	Charm() (Charm, bool, error)
	Name() string
	EndpointBindings() (Bindings, error)
}

func (a *applicationShim) Charm() (Charm, bool, error) {
	return a.Application.Charm()
}

func (a *applicationShim) EndpointBindings() (Bindings, error) {
	return a.Application.EndpointBindings()
}

type Charm interface {
	LXDProfile() *jujucharm.LXDProfile
	Revision() int
}

type Bindings interface {
	Map() map[string]string
}
