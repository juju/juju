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

type processesPersistence interface {
	ensureDefinitions(definitions ...charm.Process) error
	insert(info process.Info) error
	setStatus(id string, status process.RawStatus) error
	list(ids ...string) ([]process.Info, error)
	remove(id string) error
}

type processDefinitions struct {
	persist processesPersistence
}

func newProcessDefinitions(st *State, charm names.CharmTag) *processDefinitions {
	return &processDefinitions{
		persist: &procsPersistence{st: st, charm: charm},
	}
}

// EnsureDefined makes sure that all the provided definitions exist in
// state. So either they are there already or they get added.
func (pd processDefinitions) EnsureDefined(definitions ...charm.Process) error {
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	if err := pd.persist.ensureDefinitions(definitions...); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// TODO(ericsnow) Auto-add definitions when a charm is added.

type unitProcesses struct {
	persist processesPersistence
	unit    names.UnitTag
}

func newUnitProcesses(st *State, unit names.UnitTag, charm *names.CharmTag) *unitProcesses {
	persist := &procsPersistence{st: st, unit: unit}
	if charm != nil {
		persist.charm = *charm
	}
	return &unitProcesses{
		persist: persist,
		unit:    unit,
	}
}

// Register adds the provided process info to state.
func (ps unitProcesses) Register(info process.Info, charm names.CharmTag) error {
	if err := info.Validate(); err != nil {
		return errors.Trace(err)
	}

	if err := ps.persist.ensureDefinitions(info.Process); err != nil {
		return errors.Trace(err)
	}

	if err := ps.persist.insert(info); err != nil {
		// TODO(ericsnow) Remove the definition we may have just added?
		return errors.Trace(err)
	}
	return nil
}

// SetStatus updates the raw status for the identified process to the
// provided value.
func (ps unitProcesses) SetStatus(id string, status process.Status) error {
	if err := ps.persist.setStatus(id, status); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// List builds the list of process information for the provided process
// IDs. If none are provided then the list contains the info for all
// workload processes associated with the unit.
func (ps unitProcesses) List(ids ...string) ([]process.Info, error) {
	results, err := ps.persist.list(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// Unregister removes the identified process from state. It does not
// trigger the actual destruction of the process.
func (ps unitProcesses) Unregister(id string) error {
	err := ps.persist.remove(id)
	if errors.IsNotFound(err) {
		// We're already done!
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
