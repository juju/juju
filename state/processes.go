// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
)

// RegisterProcess registers a workload process in state.
func (st *State) RegisterProcess(unit names.UnitTag, info process.Info) error {
	charm, err := st.unitCharm(unit)
	if err != nil {
		return errors.Trace(err)
	}
	charmTag := charm.Tag().(names.CharmTag)

	ps := newUnitProcesses(st, unit, &charmTag)
	if err := ps.Register(info, charmTag); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// TODO(ericsnow) Add names.ProcessTag and use it here?

// SetProcessStatus sets the raw status of a workload process.
func (st *State) SetProcessStatus(unit names.UnitTag, id string, status process.Status) error {
	ps := newUnitProcesses(st, unit, nil)
	if err := ps.SetStatus(id, status); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ListProcesses builds the list of workload processes registered for
// the given unit and IDs. If no IDs are provided then all registered
// processes for the unit are returned.
func (st *State) ListProcesses(unit names.UnitTag, ids ...string) ([]process.Info, error) {
	ps := newUnitProcesses(st, unit, nil)
	results, err := ps.List(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// UnregisterProcess marks the identified process as unregistered.
func (st *State) UnregisterProcess(unit names.UnitTag, id string) error {
	ps := newUnitProcesses(st, unit, nil)
	if err := ps.Unregister(id); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// TODO(ericsnow) DestroyProcess: Mark the proc as Dying.
// TODO(ericsnow) We need a worker to clean up dying procs.

// defineProcesses adds the workload process definitions from the provided
// charm metadata to state.
func (st *State) defineProcesses(charmTag names.CharmTag, meta charm.Meta) error {
	var definitions []charm.Process
	for _, definition := range meta.Processes {
		definitions = append(definitions, definition)
	}
	pd := newProcessDefinitions(st, charmTag)
	if err := pd.EnsureDefined(definitions...); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// The persistence methods needed for workload processes in state.
type processesPersistence interface {
	EnsureDefinitions(definitions ...charm.Process) ([]string, []string, error)
	Insert(info process.Info) (bool, error)
	SetStatus(id string, status process.RawStatus) (bool, error)
	List(ids ...string) ([]process.Info, []string, error)
	Remove(id string) (bool, error)
}

// ProcessDefinitions provides the definition-related functionality
// needed by state.
type ProcessDefinitions struct {
	// Persist is the persistence layer that will be used.
	Persist processesPersistence
}

func newProcessDefinitions(st *State, charm names.CharmTag) *ProcessDefinitions {
	return &ProcessDefinitions{
		Persist: &procsPersistence{st: st, charm: charm},
	}
}

// EnsureDefined makes sure that all the provided definitions exist in
// state. So either they are there already or they get added.
func (pd ProcessDefinitions) EnsureDefined(definitions ...charm.Process) error {
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return errors.NewNotValid(err, "bad definition")
		}
	}
	_, mismatched, err := pd.Persist.EnsureDefinitions(definitions...)
	if err != nil {
		return errors.Trace(err)
	}
	if len(mismatched) > 0 {
		return errors.NotValidf("mismatched definitions for %v", mismatched)
	}
	return nil
}

// TODO(ericsnow) Auto-add definitions when a charm is added.

// UnitProcesses provides the functionality related to a unit's
// processes, as needed by state.
type UnitProcesses struct {
	// Persist is the persistence layer that will be used.
	Persist processesPersistence
	// Unit identifies the unit associated with the processes.
	Unit names.UnitTag
}

func newUnitProcesses(st *State, unit names.UnitTag, charm *names.CharmTag) *UnitProcesses {
	persist := &procsPersistence{st: st, unit: unit}
	if charm != nil {
		persist.charm = *charm
	}
	return &UnitProcesses{
		Persist: persist,
		Unit:    unit,
	}
}

// Register adds the provided process info to state.
func (ps UnitProcesses) Register(info process.Info, charm names.CharmTag) error {
	if err := info.Validate(); err != nil {
		return errors.NewNotValid(err, "bad process info")
	}

	_, mismatched, err := ps.Persist.EnsureDefinitions(info.Process)
	if err != nil {
		return errors.Trace(err)
	}
	if len(mismatched) > 0 {
		return errors.NotValidf("mismatched definition for %q", info.Name)
	}

	ok, err := ps.Persist.Insert(info)
	if err != nil {
		// TODO(ericsnow) Remove the definition we may have just added?
		return errors.Trace(err)
	}
	if !ok {
		// TODO(ericsnow) Remove the definition we may have just added?
		return errors.NotValidf("process %s (already in state)", info.ID())
	}

	return nil
}

// SetStatus updates the raw status for the identified process to the
// provided value.
func (ps UnitProcesses) SetStatus(id string, status process.Status) error {
	found, err := ps.Persist.SetStatus(id, status)
	if err != nil {
		return errors.Trace(err)
	}
	if !found {
		return errors.NotFoundf(id)
	}
	return nil
}

// List builds the list of process information for the provided process
// IDs. If none are provided then the list contains the info for all
// workload processes associated with the unit. Missing processes
// are ignored.
func (ps UnitProcesses) List(ids ...string) ([]process.Info, error) {
	// TODO(ericsnow) Call ListAll if ids is empty.
	results, _, err := ps.Persist.List(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(ericsnow) Ensure that the number returned matches the
	// number expected.
	return results, nil
}

// Unregister removes the identified process from state. It does not
// trigger the actual destruction of the process.
func (ps UnitProcesses) Unregister(id string) error {
	// If the record wasn't found then we're already done.
	_, err := ps.Persist.Remove(id)
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) Remove unit-based definition when no procs left.
	return nil
}
