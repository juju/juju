// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

// Machiner provides access to the Machiner API facade.
type Machiner struct {
	stcaller common.Caller
}

// MachinerMachine provides access to state.Machine methods through
// the Machiner facade.
// TODO (dimitern) This will be renamed to Machine when Machiner is
// moved into its own package in a follow-up.
type MachinerMachine struct {
	id       string
	life     params.Life
	machiner *Machiner
}

// New creates a new client-side Machiner facade.
func New(stcaller common.Caller) *Machiner {
	return &Machiner{stcaller}
}

// machineLife requests the lifecycle of the given machine from the server.
func (m *Machiner) machineLife(id string) (params.Life, error) {
	var result params.MachinesLifeResults
	args := params.Machines{
		Ids: []string{id},
	}
	err := m.stcaller.Call("Machiner", "", "Life", args, &result)
	if err != nil {
		return "", err
	}
	if len(result.Machines) != 1 {
		return "", fmt.Errorf("expected one result, got %d", len(result.Machines))
	}
	if err := result.Machines[0].Error; err != nil {
		return "", err
	}
	return result.Machines[0].Life, nil
}

// Machine provides access to methods of a state.Machine through the facade.
func (m *Machiner) Machine(id string) (*MachinerMachine, error) {
	life, err := m.machineLife(id)
	if err != nil {
		return nil, err
	}
	return &MachinerMachine{
		id:       id,
		life:     life,
		machiner: m,
	}, nil
}

// SetStatus changes the status of the machine.
func (mm *MachinerMachine) SetStatus(status params.Status, info string) error {
	var result params.ErrorResults
	args := params.MachinesSetStatus{
		Machines: []params.MachineSetStatus{
			{Id: mm.id, Status: status, Info: info},
		},
	}
	err := mm.machiner.stcaller.Call("Machiner", "", "SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.Errors[0]
}

// Refresh updates the cached local copy of the machine's data.
func (mm *MachinerMachine) Refresh() error {
	life, err := mm.machiner.machineLife(mm.id)
	if err != nil {
		return err
	}
	mm.life = life
	return nil
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (mm *MachinerMachine) EnsureDead() error {
	var result params.ErrorResults
	args := params.Machines{
		Ids: []string{mm.id},
	}
	err := mm.machiner.stcaller.Call("Machiner", "", "EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.Errors[0]
}

// Id returns the machine id.
func (mm *MachinerMachine) Id() string {
	return mm.id
}

// Life returns the machine's lifecycle value.
func (mm *MachinerMachine) Life() params.Life {
	return mm.life
}
