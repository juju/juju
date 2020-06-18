// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// StateLinkLayerDevice represents a link layer device address from state package.
type StateLinkLayerDeviceAddress interface {
	DeviceName() string
	ConfigMethod() network.AddressConfigMethod
	SubnetCIDR() string
	DNSServers() []string
	DNSSearchDomains() []string
	GatewayAddress() string
	IsDefaultGateway() bool
	Value() string

	// Origin indicates the authority that is maintaining this address.
	Origin() network.Origin

	// SetOriginOps returns the transaction operations required to change
	// the origin for this address.
	SetOriginOps(origin network.Origin) []txn.Op

	// SetProviderIDOps returns the operations required to set the input
	// provider ID for the address.
	SetProviderIDOps(id network.Id) ([]txn.Op, error)
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

	// ProviderID returns the provider-specific identifier for this device.
	ProviderID() network.Id

	// SetProviderIDOps returns the operations required to set the input
	// provider ID for the link-layer device.
	SetProviderIDOps(id network.Id) ([]txn.Op, error)
}

// StateMachine represents a machine from state package.
type StateMachine interface {
	state.Entity

	Id() string
	InstanceId() (instance.Id, error)
	ProviderAddresses() network.SpaceAddresses
	SetProviderAddresses(...network.SpaceAddress) error
	AllLinkLayerDevices() ([]StateLinkLayerDevice, error)
	AllAddresses() ([]StateLinkLayerDeviceAddress, error)
	InstanceStatus() (status.StatusInfo, error)
	SetInstanceStatus(status.StatusInfo) error
	SetStatus(status.StatusInfo) error
	String() string
	Refresh() error
	Life() state.Life
	Status() (status.StatusInfo, error)
	IsManual() (bool, error)
	AssertAliveOp() txn.Op
}

type StateInterface interface {
	state.ModelAccessor
	state.ModelMachinesWatcher
	state.EntityFinder
	network.SpaceLookup

	Machine(id string) (StateMachine, error)

	// ApplyOperation applies a given ModelOperation to the model.
	ApplyOperation(state.ModelOperation) error
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
		out[i] = dev
	}

	return out, nil
}

func (s machineShim) AllAddresses() ([]StateLinkLayerDeviceAddress, error) {
	addrList, err := s.Machine.AllAddresses()
	if err != nil {
		return nil, err
	}

	out := make([]StateLinkLayerDeviceAddress, len(addrList))
	for i, addr := range addrList {
		out[i] = addr
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
