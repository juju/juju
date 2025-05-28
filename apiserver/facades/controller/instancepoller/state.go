// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	commonnetwork "github.com/juju/juju/apiserver/common/network"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// StateMachine represents a machine from state package.
type StateMachine interface {
	state.Entity
	commonnetwork.LinkLayerMachine

	ProviderAddresses() network.SpaceAddresses
	SetProviderAddresses(controller.Config, ...network.SpaceAddress) error
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
	state.ModelMachinesWatcher
	state.EntityFinder

	Machine(id string) (StateMachine, error)

	// ApplyOperation applies a given ModelOperation to the model.
	ApplyOperation(state.ModelOperation) error
}

type machineShim struct {
	*state.Machine
}

func (s machineShim) AllLinkLayerDevices() ([]commonnetwork.LinkLayerDevice, error) {
	devList, err := s.Machine.AllLinkLayerDevices()
	if err != nil {
		return nil, err
	}

	out := make([]commonnetwork.LinkLayerDevice, len(devList))
	for i, dev := range devList {
		out[i] = dev
	}

	return out, nil
}

func (s machineShim) AllDeviceAddresses() ([]commonnetwork.LinkLayerAddress, error) {
	addrList, err := s.Machine.AllDeviceAddresses()
	if err != nil {
		return nil, err
	}

	out := make([]commonnetwork.LinkLayerAddress, len(addrList))
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

	return machineShim{Machine: m}, nil
}

var getState = func(st *state.State, m *state.Model) StateInterface {
	return stateShim{State: st, Model: m}
}
