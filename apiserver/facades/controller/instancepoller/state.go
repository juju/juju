// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// StateLinkLayerDevice represents a link layer device address from state package.
type StateLinkLayerDeviceAddress interface {
	ConfigMethod() network.AddressConfigMethod
	SubnetCIDR() string
	DNSServers() []string
	DNSSearchDomains() []string
	GatewayAddress() string
	IsDefaultGateway() bool
	Value() string
}

// StateLinkLayerDevice represents a link layer device from state package.
type StateLinkLayerDevice interface {
	Name() string
	MTU() uint
	Type() network.LinkLayerDeviceType
	IsLoopbackDevice() bool
	MACAddress() string
	IsAutoStart() bool
	IsUp() bool
	ParentName() string
	Addresses() ([]StateLinkLayerDeviceAddress, error)
}

// StateMachine represents a machine from state package.
type StateMachine interface {
	state.Entity

	Id() string
	InstanceId() (instance.Id, error)
	ProviderAddresses() network.SpaceAddresses
	SetProviderAddresses(...network.SpaceAddress) error
	AllLinkLayerDevices() ([]StateLinkLayerDevice, error)
	SetParentLinkLayerDevicesBeforeTheirChildren([]state.LinkLayerDeviceArgs) error
	SetDevicesAddressesIdempotently([]state.LinkLayerDeviceAddress) error
	InstanceStatus() (status.StatusInfo, error)
	SetInstanceStatus(status.StatusInfo) error
	SetStatus(status.StatusInfo) error
	String() string
	Refresh() error
	Life() state.Life
	Status() (status.StatusInfo, error)
	IsManual() (bool, error)
}

type StateInterface interface {
	state.ModelAccessor
	state.ModelMachinesWatcher
	state.EntityFinder
	network.SpaceLookup

	Machine(id string) (StateMachine, error)
}

type linkLayerDeviceShim struct {
	*state.LinkLayerDevice
}

func (s linkLayerDeviceShim) Addresses() ([]StateLinkLayerDeviceAddress, error) {
	addrList, err := s.LinkLayerDevice.Addresses()
	if err != nil {
		return nil, err
	}

	out := make([]StateLinkLayerDeviceAddress, len(addrList))
	for i, addr := range addrList {
		out[i] = addr
	}

	return out, nil
}

type machineShim struct {
	*state.Machine
}

func (s machineShim) AllLinkLayerDevices() ([]StateLinkLayerDevice, error) {
	devList, err := s.Machine.AllLinkLayerDevices()
	if err != nil {
		return nil, err
	}

	out := make([]StateLinkLayerDevice, len(devList))
	for i, dev := range devList {
		out[i] = linkLayerDeviceShim{dev}
	}

	return out, nil
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

func (s stateShim) Machine(id string) (StateMachine, error) {
	m, err := s.State.Machine(id)
	if err != nil {
		return nil, err
	}

	return machineShim{m}, nil
}

var getState = func(st *state.State, m *state.Model) StateInterface {
	return stateShim{st, m}
}
