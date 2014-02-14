// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// Machine represents a juju machine as seen by a machiner worker.
type Machine struct {
	tag  string
	life params.Life
	st   *State
}

// Tag returns the machine's tag.
func (m *Machine) Tag() string {
	return m.tag
}

// Life returns the machine's lifecycle value.
func (m *Machine) Life() params.Life {
	return m.life
}

// Refresh updates the cached local copy of the machine's data.
func (m *Machine) Refresh() error {
	life, err := m.st.machineLife(m.tag)
	if err != nil {
		return err
	}
	m.life = life
	return nil
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(status params.Status, info string, data params.StatusData) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.SetEntityStatus{
			{Tag: m.tag, Status: status, Info: info, Data: data},
		},
	}
	err := m.st.caller.Call("Machiner", "", "SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// SetMachineAddresses sets the machine determined addresses of the machine.
func (m *Machine) SetMachineAddresses(addresses []instance.Address) error {
	var result params.ErrorResults
	args := params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{
			{Tag: m.Tag(), Addresses: addresses},
		},
	}
	err := m.st.caller.Call("Machiner", "", "SetMachineAddresses", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (m *Machine) EnsureDead() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag}},
	}
	err := m.st.caller.Call("Machiner", "", "EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Watch returns a watcher for observing changes to the machine.
func (m *Machine) Watch() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag}},
	}
	err := m.st.caller.Call("Machiner", "", "Watch", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(m.st.caller, result)
	return w, nil
}
