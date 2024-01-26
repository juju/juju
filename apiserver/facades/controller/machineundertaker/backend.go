// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// Backend defines the methods the machine undertaker needs from
// state.State.
type Backend interface {
	// AllMachineRemovals returns all of the machines which have been
	// marked for removal.
	AllMachineRemovals() ([]string, error)

	// CompleteMachineRemovals removes the machines (and the associated removal
	// requests) after the provider-level cleanup is done.
	CompleteMachineRemovals(machineIDs ...string) error

	// WatchMachineRemovals returns a NotifyWatcher that triggers
	// whenever machine removal requests are added or removed.
	WatchMachineRemovals() state.NotifyWatcher

	// Machine gets a specific machine, so we can collect details of
	// its network interfaces.
	Machine(id string) (Machine, error)
}

// Machine defines the methods we need from state.Machine.
type Machine interface {
	// AllProviderInterfaceInfos returns the details needed to talk to
	// the provider about this machine's attached devices.
	AllProviderInterfaceInfos() ([]network.ProviderInterfaceInfo, error)
}

type backendShim struct {
	*state.State
}

// Machine implements Machine.
func (b *backendShim) Machine(id string) (Machine, error) {
	return b.State.Machine(id)
}
