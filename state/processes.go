// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
)

// RegisterProcess registers a workload process in state.
func (st *State) RegisterProcess(unit names.UnitTag, info process.Info) error {
	ps := newUnitProcesses(st, unit)
	charm, err := st.unitCharm(unit)
	if err != nil {
		return errors.Trace(err)
	}
	charmTag := charm.Tag().(names.CharmTag)
	if err := ps.register(charmTag, info); err != nil {
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
	charm   names.CharmTag
	unit    names.UnitTag
}

func newProcessDefinitions(st *State, charm names.CharmTag) *processDefinitions {
	return &processDefinitions{
		persist: &processesPersistence{st: st},
		charm:   charm,
	}
}

func (pd processDefinitions) resolve(name string) string {
	// The URL will always parse successfully.
	charmURL, _ := charm.ParseURL(pd.charm.Id())
	return fmt.Sprintf("%s#%s", charmGlobalKey(charmURL), name)
}

func (pd processDefinitions) ensureDefined(definitions ...charm.Process) error {
	var ids []string
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return errors.Trace(err)
		}
		ids = append(ids, pd.resolve(definition.Name))
	}
	if err := pd.persist.ensureDefinitions(ids, definitions, pd.unit.Id()); err != nil {
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
		persist: &processesPersistence{st: st},
		unit:    unit,
	}
}

func (ps unitProcesses) resolve(id string) string {
	return fmt.Sprintf("%s#%s", unitGlobalKey(ps.unit.Id()), id)
}

func (ps unitProcesses) register(charm names.CharmTag, info process.Info) error {
	if err := info.Validate(); err != nil {
		return errors.Trace(err)
	}

	pd := processDefinitions{
		persist: ps.persist,
		charm:   charm,
		unit:    ps.unit,
	}
	if err := pd.ensureDefined(info.Process); err != nil {
		return errors.Trace(err)
	}

	id := ps.resolve(info.Name)
	if err := ps.persist.insert(id, charm.Id(), info); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (ps unitProcesses) setStatus(id string, status process.Status) error {
	id = ps.resolve(id)
	if err := ps.persist.setStatus(id, status); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (ps unitProcesses) list(ids ...string) ([]process.Info, error) {
	for i, id := range ids {
		ids[i] = ps.resolve(id)
	}
	results, err := ps.persist.list(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

func (ps unitProcesses) unregister(id string) error {
	id = ps.resolve(id)
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

type processesPersistenceBase interface {
	run(transactions jujutxn.TransactionSource) error
}

type processesPersistence struct {
	st processesPersistenceBase
}

func (pp processesPersistence) ensureDefinitions(ids []string, definitions []charm.Process, unit string) error {
	// Add definition if not already added (or ensure matches).

	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (pp processesPersistence) insert(id, charm string, info process.Info) error {
	// Ensure defined.

	// Add launch info.
	// Add process info.

	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (pp processesPersistence) setStatus(id string, status process.Status) error {
	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (pp processesPersistence) list(ids ...string) ([]process.Info, error) {
	// TODO(ericsnow) finish!
	return nil, errors.Errorf("not finished")
}

func (pp processesPersistence) remove(id string) error {
	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}
