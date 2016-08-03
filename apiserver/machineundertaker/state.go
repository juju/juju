// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// State defines the methods the machine undertaker needs from
// state.State.
type State interface {

	// AllMachineRemovals returns all of the machine removals that
	// need processing.
	AllMachineRemovals() ([]MachineRemoval, error)

	// ClearMachineRemovals removes all of the specified machine
	// removals (which have presumably been processed).
	ClearMachineRemovals(machineIDs []string) error

	// WatchMachineRemovals returns a NotifyWatcher that triggers
	// whenever machine removal requests are added or removed.
	WatchMachineRemovals() state.NotifyWatcher
}

type stateShim struct {
	*state.State
}

func (s *stateShim) AllMachineRemovals() ([]MachineRemoval, error) {
	stateRemovals, err := s.State.AllMachineRemovals()
	if err != nil {
		return nil, errors.Trace(err)
	}
	removals := make([]MachineRemoval, len(stateRemovals))
	for i := range stateRemovals {
		removals[i] = &machineRemovalShim{stateRemovals[i]}
	}
	return removals, nil
}

type MachineRemoval interface {
	MachineID() string
	LinkLayerDevices() []LinkLayerDevice
}

type machineRemovalShim struct {
	*state.MachineRemoval
}

func (s *machineRemovalShim) LinkLayerDevices() []LinkLayerDevice {
	var result []LinkLayerDevice
	for _, device := range s.MachineRemoval.LinkLayerDevices() {
		result = append(result, device)
	}
	return result
}

type LinkLayerDevice interface {
	Name() string
	MACAddress() string
	Type() state.LinkLayerDeviceType
	MTU() uint
	ProviderID() network.Id
}
