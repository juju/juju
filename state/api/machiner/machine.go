// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// Machine represents a juju machine as seen by a machiner worker.
type Machine struct {
	tag    string
	life   params.Life
	mstate *Machiner
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
	life, err := m.mstate.machineLife(m.tag)
	if err != nil {
		return err
	}
	m.life = life
	return nil
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(status params.Status, info string) error {
	var result params.ErrorResults
	args := params.MachinesSetStatus{
		Machines: []params.MachineSetStatus{
			{Tag: m.tag, Status: status, Info: info},
		},
	}
	err := m.mstate.caller.Call("Machiner", "", "SetStatus", args, &result)
	if err != nil {
		return err
	}
	if len(result.Errors) != 1 {
		return fmt.Errorf("expected one result, got %d", len(result.Errors))
	}
	return result.Errors[0]
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (m *Machine) EnsureDead() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag}},
	}
	err := m.mstate.caller.Call("Machiner", "", "EnsureDead", args, &result)
	if err != nil {
		return err
	}
	if len(result.Errors) != 1 {
		return fmt.Errorf("expected one result, got %d", len(result.Errors))
	}
	return result.Errors[0]
}

// Watch returns a watcher for observing changes to the machine.
func (m *Machine) Watch() (*watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag}},
	}
	err := m.mstate.caller.Call("Machiner", "", "Watch", args, &results)
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
	w := watcher.NewNotifyWatcher(m.mstate.caller, result)
	return w, nil
}
