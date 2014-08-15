// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api/common"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
)

// Machine represents a juju machine as seen by a machiner worker.
type Machine struct {
	tag  names.MachineTag
	life params.Life
	st   *State
}

// Tag returns the machine's tag.
func (m *Machine) Tag() names.Tag {
	return m.tag
}

// Life returns the machine's lifecycle value.
func (m *Machine) Life() params.Life {
	return m.life
}

// IsManual returns true if the machine was manually provisioned.
func (m *Machine) IsManual() (bool, error) {
	var result params.IsManualResults
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: m.tag.String()},
		},
	}
	err := m.st.facade.FacadeCall("IsManual", args, &result)
	if err != nil {
		return false, err
	}
	if n := len(result.Results); n != 1 {
		return false, fmt.Errorf("expected 1 result, got %d", n)
	}
	// TODO(mue) Add check of possible results error.
	return result.Results[0].IsManual, nil
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
		Entities: []params.EntityStatus{
			{Tag: m.tag.String(), Status: status, Info: info, Data: data},
		},
	}
	err := m.st.facade.FacadeCall("SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// SetMachineAddresses sets the machine determined addresses of the machine.
func (m *Machine) SetMachineAddresses(addresses []network.Address) error {
	var result params.ErrorResults
	args := params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{
			{Tag: m.Tag().String(), Addresses: addresses},
		},
	}
	err := m.st.facade.FacadeCall("SetMachineAddresses", args, &result)
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
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Watch returns a watcher for observing changes to the machine.
func (m *Machine) Watch() (watcher.NotifyWatcher, error) {
	return common.Watch(m.st.facade, m.tag)
}
