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

	ps := newUnitProcesses(st, unit)
	if err := ps.register(info, charmTag); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// TODO(ericsnow) Add names.ProcessTag and use it here?

// SetProcessStatus sets the raw status of a workload process.
func (st *State) SetProcessStatus(unit names.UnitTag, id string, status process.Status) error {
	ps := newUnitProcesses(st, unit)
	if err := ps.setStatus(id, status); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ListProcesses builds the list of workload processes registered for
// the given unit and IDs. If no IDs are provided then all registered
// processes for the unit are returned.
func (st *State) ListProcesses(unit names.UnitTag, ids ...string) ([]process.Info, error) {
	ps := newUnitProcesses(st, unit)
	results, err := ps.list(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// UnregisterProcess marks the identified process as unregistered.
func (st *State) UnregisterProcess(unit names.UnitTag, id string) error {
	ps := newUnitProcesses(st, unit)
	if err := ps.unregister(id); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// defineProcesses adds the workload process definitions from the provided
// charm metadata to state.
func (st *State) defineProcesses(charmTag names.CharmTag, meta charm.Meta) error {
	var definitions []charm.Process
	for _, definition := range meta.Processes {
		definitions = append(definitions, definition)
	}
	pd := newProcessDefinitions(st, charmTag)
	if err := pd.ensureDefined(definitions...); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type processDefinitions struct {
	persist *processesPersistence
}

func newProcessDefinitions(st *State, charm names.CharmTag) *processDefinitions {
	return &processDefinitions{
		persist: &processesPersistence{st: st, charm: charm},
	}
}

func (pd processDefinitions) ensureDefined(definitions ...charm.Process) error {
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
	persist *processesPersistence
	unit    names.UnitTag
}

func newUnitProcesses(st *State, unit names.UnitTag) *unitProcesses {
	return &unitProcesses{
		persist: &processesPersistence{st: st, unit: unit},
		unit:    unit,
	}
}

func (ps unitProcesses) register(info process.Info, charm names.CharmTag) error {
	if err := info.Validate(); err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Use a safer mechanism,
	// e.g. pass charm to ensureDefinitions?
	ps.persist.charm = charm
	if err := ps.persist.ensureDefinitions(info.Process); err != nil {
		return errors.Trace(err)
	}

	if err := ps.persist.insert(info); err != nil {
		// TODO(ericsnow) Remove the definition we may have just added?
		return errors.Trace(err)
	}
	return nil
}

func (ps unitProcesses) setStatus(id string, status process.Status) error {
	if err := ps.persist.setStatus(id, status); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (ps unitProcesses) list(ids ...string) ([]process.Info, error) {
	results, err := ps.persist.list(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

func (ps unitProcesses) unregister(id string) error {
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
